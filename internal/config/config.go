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

type Config struct {
	Auths        map[string]RegistryAuth `yaml:"auths"`
	Experimental struct {
		Enabled bool   `yaml:"enabled"`
		Repo    string `yaml:"repo"`
	} `yaml:"experimental"`
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
	globalConfig.Experimental.Enabled = true
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

func ExperimentalEnabled() bool {
	if globalConfig == nil {
		return true
	}
	return globalConfig.Experimental.Enabled
}

func ExperimentalRepo() (string, error) {
	if globalConfig == nil || globalConfig.Experimental.Repo == "" {
		return "", fmt.Errorf("experimental repository not configured: set experimental.repo in config file")
	}
	repo := globalConfig.Experimental.Repo
	if strings.Contains(repo, "@") {
		return "", fmt.Errorf("repository must not be a digest reference (contains '@')")
	}
	lastColon := strings.LastIndex(repo, ":")
	lastSlash := strings.LastIndex(repo, "/")
	if lastColon > lastSlash {
		return "", fmt.Errorf("repository must not include a tag (found ':' after last '/')")
	}
	return repo, nil
}
