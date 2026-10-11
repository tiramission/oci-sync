package xdg

import (
	"os"
	"path/filepath"
)

// homeDir resolves the user's home directory. The HOME environment variable
// is not set on Windows, so fall back to os.UserHomeDir (USERPROFILE) and
// then to os.TempDir. Without the fallbacks every derived path would become
// relative to the working directory and config would be written next to
// whatever directory the tool happens to run in.
func homeDir() string {
	if dir := os.Getenv("HOME"); dir != "" {
		return dir
	}
	if dir, err := os.UserHomeDir(); err == nil && dir != "" {
		return dir
	}
	return os.TempDir()
}

func BaseDir(envKey, defaultSubdir string) string {
	if dir := os.Getenv(envKey); dir != "" {
		return dir
	}
	return filepath.Join(homeDir(), defaultSubdir)
}

func ConfigDir() string {
	return BaseDir("XDG_CONFIG_HOME", ".config")
}

func CacheDir() string {
	return BaseDir("XDG_CACHE_HOME", ".cache")
}

func DataDir() string {
	return BaseDir("XDG_DATA_HOME", ".local/share")
}
