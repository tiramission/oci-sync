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

func TestPackUnpackFile(t *testing.T) {
	// Create a temp file
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "hello.txt")
	if err := os.WriteFile(srcFile, []byte("hello, oci-sync!"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Pack
	data, err := archive.Pack(srcFile)
	if err != nil {
		t.Fatalf("Pack error: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("Pack returned empty data")
	}

	// Unpack
	destDir := filepath.Join(tmpDir, "out")
	if err := archive.Unpack(data, destDir); err != nil {
		t.Fatalf("Unpack error: %v", err)
	}

	// Verify
	got, err := os.ReadFile(filepath.Join(destDir, "hello.txt"))
	if err != nil {
		t.Fatalf("read unpacked file: %v", err)
	}
	if string(got) != "hello, oci-sync!" {
		t.Errorf("content mismatch: got %q", string(got))
	}
}

func TestPackUnpackDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a directory with several files
	srcDir := filepath.Join(tmpDir, "mydir")
	if err := os.MkdirAll(filepath.Join(srcDir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"a.txt":     "file A",
		"b.txt":     "file B",
		"sub/c.txt": "file C in sub",
	}
	for name, content := range files {
		path := filepath.Join(srcDir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Pack
	data, err := archive.Pack(srcDir)
	if err != nil {
		t.Fatalf("Pack error: %v", err)
	}

	// Unpack
	destDir := filepath.Join(tmpDir, "out")
	if err := archive.Unpack(data, destDir); err != nil {
		t.Fatalf("Unpack error: %v", err)
	}

	// Verify
	for name, want := range files {
		got, err := os.ReadFile(filepath.Join(destDir, "mydir", name))
		if err != nil {
			t.Errorf("read %s: %v", name, err)
			continue
		}
		if string(got) != want {
			t.Errorf("content mismatch for %s: got %q, want %q", name, string(got), want)
		}
	}
}

// TestPackFollowsSymlinks pins the symlink policy: links are stored as
// ordinary files holding the target's content. Storing the link target
// instead is not an option — Unpack materialises only regular files and
// directories, so every link would silently come back empty.
func TestPackFollowsSymlinks(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "d")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "real.txt"), []byte("target content"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real.txt", filepath.Join(srcDir, "link.txt")); err != nil {
		t.Skipf("symlinks unsupported on this host: %v", err)
	}

	data, err := archive.Pack(srcDir)
	if err != nil {
		t.Fatalf("Pack error: %v", err)
	}

	destDir := filepath.Join(tmpDir, "out")
	if err := archive.Unpack(data, destDir); err != nil {
		t.Fatalf("Unpack error: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(destDir, "d", "link.txt"))
	if err != nil {
		t.Fatalf("read unpacked link: %v", err)
	}
	if string(got) != "target content" {
		t.Errorf("symlink round trip = %q, want the target's content", got)
	}
}

// TestPackSkipsSymlinkedDirectories keeps a link to a directory from
// duplicating the whole subtree under a second name.
func TestPackSkipsSymlinkedDirectories(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "d")
	if err := os.MkdirAll(filepath.Join(srcDir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "sub", "f.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("sub", filepath.Join(srcDir, "dirlink")); err != nil {
		t.Skipf("symlinks unsupported on this host: %v", err)
	}

	data, err := archive.Pack(srcDir)
	if err != nil {
		t.Fatalf("Pack error: %v", err)
	}

	destDir := filepath.Join(tmpDir, "out")
	if err := archive.Unpack(data, destDir); err != nil {
		t.Fatalf("Unpack error: %v", err)
	}

	if _, err := os.Lstat(filepath.Join(destDir, "d", "dirlink")); !os.IsNotExist(err) {
		t.Errorf("symlinked directory should be skipped, Lstat err = %v", err)
	}
	// The real directory must still be present.
	if _, err := os.Stat(filepath.Join(destDir, "d", "sub", "f.txt")); err != nil {
		t.Errorf("real subdirectory missing: %v", err)
	}
}

// TestUnpackRejectsPathTraversal keeps a hostile archive from writing
// outside destPath. Only relative escapes matter: filepath.Join already
// folds a leading "/" into destPath, so "/abs/x" lands inside dest and is
// safe rather than rejected.
func TestUnpackRejectsPathTraversal(t *testing.T) {
	cases := []string{
		"../escape.txt",
		"a/../../escape.txt",
		"../../escape.txt",
	}
	if os.PathSeparator == '\\' {
		// A member name carrying a Windows separator is a traversal vector
		// that a Unix-built archive could never produce.
		cases = append(cases, `..\escape.txt`, `a\..\..\escape.txt`)
	}

	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			gw := gzip.NewWriter(&buf)
			tw := tar.NewWriter(gw)
			body := []byte("pwned")
			if err := tw.WriteHeader(&tar.Header{
				Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg,
			}); err != nil {
				t.Fatal(err)
			}
			if _, err := tw.Write(body); err != nil {
				t.Fatal(err)
			}
			if err := tw.Close(); err != nil {
				t.Fatal(err)
			}
			if err := gw.Close(); err != nil {
				t.Fatal(err)
			}

			destDir := t.TempDir()
			if err := archive.Unpack(buf.Bytes(), destDir); err == nil {
				t.Errorf("Unpack(%q) = nil, want a path traversal error", name)
			}
		})
	}
}

// TestUnpackKeepsLeadingSlashInsideDest documents that an absolute-looking
// member name is contained rather than rejected.
func TestUnpackKeepsLeadingSlashInsideDest(t *testing.T) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	body := []byte("contained")
	if err := tw.WriteHeader(&tar.Header{
		Name: "/abs/escape.txt", Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}

	destDir := t.TempDir()
	if err := archive.Unpack(buf.Bytes(), destDir); err != nil {
		t.Fatalf("Unpack error: %v", err)
	}

	got := filepath.Join(destDir, "abs", "escape.txt")
	if b, err := os.ReadFile(got); err != nil || string(b) != "contained" {
		t.Errorf("file not written inside dest at %s: %v", got, err)
	}
}

// TestPackUsesSlashSeparatedMemberNames pins the tar wire format: member
// names must use forward slashes on every host OS. filepath.Join emits
// backslashes on Windows, which silently collapses the tree into flat names
// when the archive is unpacked on Linux. A pack-then-unpack round trip on a
// single OS cannot catch this, so inspect the headers directly.
func TestPackUsesSlashSeparatedMemberNames(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "mydir")
	if err := os.MkdirAll(filepath.Join(srcDir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"a.txt":     "file A",
		"sub/b.txt": "file B in sub",
	} {
		if err := os.WriteFile(filepath.Join(srcDir, filepath.FromSlash(name)), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	data, err := archive.Pack(srcDir)
	if err != nil {
		t.Fatalf("Pack error: %v", err)
	}

	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	defer gr.Close()

	var names []string
	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read tar entry: %v", err)
		}
		names = append(names, hdr.Name)
	}

	want := map[string]bool{"mydir/": true, "mydir/a.txt": true, "mydir/sub/": true, "mydir/sub/b.txt": true}
	for _, n := range names {
		if strings.Contains(n, `\`) {
			t.Errorf("member name %q contains a backslash; tar names must be slash-separated", n)
		}
		if !want[n] {
			t.Errorf("unexpected member name %q", n)
		}
		delete(want, n)
	}
	for n := range want {
		t.Errorf("missing member name %q", n)
	}
}
