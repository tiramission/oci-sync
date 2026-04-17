package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	ConfigFileName = "oci-sync"
)

type RegistryAuth struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type Shortcut struct {
	Repo string `yaml:"repo"`
}

type Config struct {
	Auths     map[string]RegistryAuth `yaml:"auths"`
	Shortcuts map[string]Shortcut     `yaml:"shortcuts"`
}

var globalConfig *Config

func InitConfig() error {
	paths := []string{
		".",
		filepath.Join(os.Getenv("HOME"), ".config", "oci-sync"),
	}

	for _, dir := range paths {
		configPath := filepath.Join(dir, ConfigFileName+".yaml")
		data, err := os.ReadFile(configPath)
		if err == nil {
			var cfg Config
			if err := yaml.Unmarshal(data, &cfg); err != nil {
				return fmt.Errorf("parse config %s: %w", configPath, err)
			}
			globalConfig = &cfg
			return nil
		}
	}

	globalConfig = &Config{Auths: make(map[string]RegistryAuth)}
	return nil
}

func ConfigFileUsed() string {
	paths := []string{
		".",
		filepath.Join(os.Getenv("HOME"), ".config", "oci-sync"),
	}

	for _, dir := range paths {
		configPath := filepath.Join(dir, ConfigFileName+".yaml")
		if _, err := os.Stat(configPath); err == nil {
			return configPath
		}
	}
	return ""
}

func GetRegistryAuth(host string) (RegistryAuth, bool) {
	if globalConfig == nil {
		return RegistryAuth{}, false
	}
	auth, ok := globalConfig.Auths[host]
	return auth, ok
}

func GetShortcutRepo(name string) (string, error) {
	if globalConfig == nil {
		return "", fmt.Errorf("config not initialized")
	}
	shortcut, ok := globalConfig.Shortcuts[name]
	if !ok {
		return "", fmt.Errorf("shortcut %q not found: add shortcuts.%s.repo to config", name, name)
	}
	repo := shortcut.Repo
	if repo == "" {
		return "", fmt.Errorf("shortcut %q repo is empty", name)
	}
	if strings.Contains(repo, "@") {
		return "", fmt.Errorf("shortcut %q repository must not be a digest reference (contains '@')", name)
	}
	lastColon := strings.LastIndex(repo, ":")
	lastSlash := strings.LastIndex(repo, "/")
	if lastColon > lastSlash {
		return "", fmt.Errorf("shortcut %q repository must not include a tag (found ':' after last '/')", name)
	}
	return repo, nil
}

func ShortcutNames() []string {
	if globalConfig == nil || globalConfig.Shortcuts == nil {
		return nil
	}
	names := make([]string, 0, len(globalConfig.Shortcuts))
	for name := range globalConfig.Shortcuts {
		names = append(names, name)
	}
	return names
}
