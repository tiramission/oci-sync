package xdg

import (
	"path/filepath"
	"testing"
)

func TestBaseDirPrefersEnvOverride(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", filepath.Join("custom", "cfg"))
	if got := ConfigDir(); got != filepath.Join("custom", "cfg") {
		t.Errorf("ConfigDir() = %q, want the XDG_CONFIG_HOME override", got)
	}
}

// Windows does not set HOME, so the fallback must reach a real absolute path.
// A relative result would place config and cache next to the working
// directory, where alias add would write a file alias list never reads.
func TestBaseDirIsAbsoluteWithoutHome(t *testing.T) {
	t.Setenv("HOME", "")
	for _, tc := range []struct {
		name string
		fn   func() string
		env  string
	}{
		{"ConfigDir", ConfigDir, "XDG_CONFIG_HOME"},
		{"CacheDir", CacheDir, "XDG_CACHE_HOME"},
		{"DataDir", DataDir, "XDG_DATA_HOME"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(tc.env, "")
			got := tc.fn()
			if !filepath.IsAbs(got) {
				t.Errorf("%s() = %q, want an absolute path", tc.name, got)
			}
		})
	}
}
