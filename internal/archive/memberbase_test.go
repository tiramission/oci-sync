package archive

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

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

// TestMemberBase pins the wrapper name for every shape of input, including
// the dot paths that cannot be reached through Pack on a locked-down host.
// Testing the helper directly is what makes the rule checkable: on Windows
// packing a volume root dies on an access-denied system directory long
// before the name is consulted, so a Pack-level test there would pass for
// the wrong reason.
func TestMemberBase(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "data")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "solo.txt")

	t.Run("regular dir", func(t *testing.T) {
		got, err := memberBase(dir, true)
		if err != nil || got != "data" {
			t.Errorf("memberBase(%q, true) = %q, %v; want \"data\", nil", dir, got, err)
		}
	})

	t.Run("regular file", func(t *testing.T) {
		got, err := memberBase(file, false)
		if err != nil || got != "solo.txt" {
			t.Errorf("memberBase(%q, false) = %q, %v; want \"solo.txt\", nil", file, got, err)
		}
	})

	// The two dot paths, whose real names only appear once resolved against
	// the working directory — so the test has to set one.
	t.Run("dot", func(t *testing.T) {
		chdir(t, dir)
		got, err := memberBase(".", true)
		if err != nil {
			t.Fatalf("memberBase(\".\", true): %v", err)
		}
		if got != filepath.Base(dir) {
			t.Errorf("memberBase(\".\", true) = %q, want %q", got, filepath.Base(dir))
		}
	})

	t.Run("dotdot", func(t *testing.T) {
		sub := filepath.Join(dir, "sub")
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		chdir(t, sub)
		got, err := memberBase("..", true)
		if err != nil {
			t.Fatalf("memberBase(\"..\", true): %v", err)
		}
		if got != filepath.Base(dir) {
			t.Errorf("memberBase(\"..\", true) = %q, want %q", got, filepath.Base(dir))
		}
	})

	// A regular file cannot be a dot path, but memberBase is handed the
	// path before anything has confirmed that, so it must still refuse.
	t.Run("dot as file", func(t *testing.T) {
		if _, err := memberBase(".", false); err == nil {
			t.Error(`memberBase(".", false) = nil error, want a refusal`)
		}
	})
	t.Run("dotdot as file", func(t *testing.T) {
		if _, err := memberBase("..", false); err == nil {
			t.Error(`memberBase("..", false) = nil error, want a refusal`)
		}
	})
}

// TestMemberBaseRefusesRoots covers the paths with no name to fall back on.
// Base("/") is "/" on Unix and "\" for "C:\", so the check has to recognise
// a separator-only name rather than the two literal spellings.
func TestMemberBaseRefusesRoots(t *testing.T) {
	cases := []string{"/"}
	if runtime.GOOS == "windows" {
		vol := filepath.VolumeName(t.TempDir())
		cases = append(cases, vol+`\`, vol+"/")
	}

	for _, root := range cases {
		t.Run(root, func(t *testing.T) {
			if _, err := memberBase(root, true); err == nil {
				t.Errorf("memberBase(%q, true) = nil error, want a refusal", root)
			}
		})
	}
}
