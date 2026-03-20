package oci

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseOCIRef(t *testing.T) {
	tests := []struct {
		ref string
		img string
		tag string
	}{
		{"oci://myrepo/myimage:1.0", "myrepo/myimage", "1.0"},
		{"oci://myrepo/myimage", "myrepo/myimage", "latest"},
		{"oci://internal.183867412.xyz:5000/test/oci-sync:1.0", "internal.183867412.xyz:5000/test/oci-sync", "1.0"},
	}
	for _, tt := range tests {
		image, tag, err := parseOCIRef(tt.ref)
		if err != nil {
			t.Fatalf("parseOCIRef(%q) error: %v", tt.ref, err)
		}
		if image != tt.img || tag != tt.tag {
			t.Fatalf("parseOCIRef(%q) = %q:%q, want %q:%q", tt.ref, image, tag, tt.img, tt.tag)
		}
	}
}

func TestIsRegistryRef(t *testing.T) {
	tests := []struct {
		image string
		want  bool
	}{
		{"myrepo/myimage", false},
		{"internal.183867412.xyz:5000/test/oci-sync", true},
		{"localhost/myimage", true},
	}
	for _, tt := range tests {
		if got := isRegistryRef(tt.image); got != tt.want {
			t.Fatalf("isRegistryRef(%q) = %v, want %v", tt.image, got, tt.want)
		}
	}
}

func TestPushPullLocal(t *testing.T) {
	tmpSrc := t.TempDir()
	tmpDest := t.TempDir()
	srcDir := filepath.Join(tmpSrc, "data")
	if err := os.Mkdir(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "hello.txt"), []byte("world"), 0o644); err != nil {
		t.Fatal(err)
	}
	ref := "oci://myrepo/myimage:2.0"
	if err := Push(srcDir, ref); err != nil {
		t.Fatal(err)
	}
	if err := Pull(ref, tmpDest); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(tmpDest, "data", "hello.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "world" {
		t.Fatalf("unexpected file content: %q", string(got))
	}
}
