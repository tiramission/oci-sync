package archive_test

import (
	"os"
	"path/filepath"
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
