package xdg

import (
	"os"
	"path/filepath"
)

func BaseDir(envKey, defaultSubdir string) string {
	if dir := os.Getenv(envKey); dir != "" {
		return dir
	}
	return filepath.Join(os.Getenv("HOME"), defaultSubdir)
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
