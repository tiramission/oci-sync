package archive_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tiramission/oci-sync/internal/archive"
)

// packMembers returns the tar member names and contents of a packed path.
func packMembers(t *testing.T, srcPath string) ([]string, map[string]string) {
	t.Helper()
	var buf bytes.Buffer
	if err := archive.Pack(&buf, srcPath); err != nil {
		t.Fatalf("Pack(%q): %v", srcPath, err)
	}
	return readMembers(t, buf.Bytes())
}

func readMembers(t *testing.T, data []byte) ([]string, map[string]string) {
	t.Helper()
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer gr.Close()

	var names []string
	bodies := map[string]string{}
	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, hdr.Name)
		if hdr.Typeflag == tar.TypeReg {
			body, err := io.ReadAll(tr)
			if err != nil {
				t.Fatal(err)
			}
			bodies[hdr.Name] = string(body)
		}
	}
	return names, bodies
}

// chdir moves into dir for the duration of the test.
func chdir(t *testing.T, dir string) {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(cwd) })
}

// treeFixture builds dir/{a.txt,sub/b.txt} and returns dir.
func treeFixture(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "b.txt"), []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestPackOfDotPathsRoundTrips pins the wrapper name for the paths a user is
// most likely to type by hand. filepath.Base answers "." for the working
// directory and ".." for its parent, which used to write member names like
// "../a.txt": the push succeeded, because writing a tar rejects nothing, and
// every pull then failed with "illegal file path in archive". A push that
// cannot be pulled back is not a push.
func TestPackOfDotPathsRoundTrips(t *testing.T) {
	root := t.TempDir()
	data := treeFixture(t, filepath.Join(root, "data"))
	sub := filepath.Join(data, "sub")

	for _, tc := range []struct {
		name string
		src  string
		// want is the wrapper directory the members must sit under.
		want string
	}{
		// Base(".") is "."; the real name is the fixture's.
		{"dot", ".", filepath.Base(data)},
		// Base("..") is ".."; from sub/ that resolves back to data.
		{"dotdot", "..", filepath.Base(data)},
		{"explicit relative", "./data", "data"},
		{"trailing slash", "data/", "data"},
		{"bare name", "data", "data"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// "." is the working directory and ".." only lands on the
			// fixture from inside it; the named paths need the parent.
			if tc.src == "." {
				chdir(t, data)
			} else if tc.src == ".." {
				chdir(t, sub)
			} else {
				chdir(t, root)
			}

			names, _ := packMembers(t, tc.src)
			for _, n := range names {
				if !strings.HasPrefix(n, tc.want+"/") {
					t.Errorf("member %q is not under %q/", n, tc.want)
				}
				// The specific failure this test exists for.
				if strings.HasPrefix(n, "../") || n == ".." || strings.HasPrefix(n, "./") {
					t.Errorf("member %q escapes or degenerates at the archive root", n)
				}
			}

			// The real check: it has to come back.
			var buf bytes.Buffer
			if err := archive.Pack(&buf, tc.src); err != nil {
				t.Fatalf("Pack: %v", err)
			}
			dest := filepath.Join(root, "out", tc.name)
			if err := archive.UnpackBytes(buf.Bytes(), dest); err != nil {
				t.Fatalf("Unpack of Pack(%q) failed, so the push could never be pulled: %v", tc.src, err)
			}
			got, err := os.ReadFile(filepath.Join(dest, tc.want, "a.txt"))
			if err != nil {
				t.Fatalf("read unpacked a.txt: %v", err)
			}
			if string(got) != "a" {
				t.Errorf("a.txt = %q, want %q", got, "a")
			}
			if _, err := os.Stat(filepath.Join(dest, tc.want, "sub", "b.txt")); err != nil {
				t.Errorf("nested file missing: %v", err)
			}
		})
	}
}

// TestPackSingleFileKeepsItsName guards the other branch of memberBase: a
// regular file must be unaffected by the dot-path handling.
func TestPackSingleFileKeepsItsName(t *testing.T) {
	root := t.TempDir()
	f := filepath.Join(root, "solo.txt")
	if err := os.WriteFile(f, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	chdir(t, root)

	names, bodies := packMembers(t, "solo.txt")
	if len(names) != 1 || names[0] != "solo.txt" {
		t.Fatalf("members = %v, want [solo.txt]", names)
	}
	if bodies["solo.txt"] != "hello" {
		t.Errorf("body = %q, want %q", bodies["solo.txt"], "hello")
	}
}
