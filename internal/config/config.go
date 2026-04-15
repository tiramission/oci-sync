package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
)

const (
	// ConfigFileName is the default config file name (without extension)
	ConfigFileName = "oci-sync"
	// ConfigFileType is the default config file type
	ConfigFileType = "yaml"

	// keyExperimentalRepo is the config key for experimental repository
	keyExperimentalRepo = "experimental.repo"
	// keyExperimentalEnabled is the config key for experimental commands enable/disable
	keyExperimentalEnabled = "experimental.enabled"
)

// ExperimentalEnabled returns whether experimental commands are enabled.
// Environment variable OCI_SYNC_EXPERIMENTAL_ENABLED takes precedence over config file.
// Default is true (enabled).
func ExperimentalEnabled() bool {
	// Check environment variable first (takes precedence)
	if env := os.Getenv("OCI_SYNC_EXPERIMENTAL_ENABLED"); env != "" {
		return strings.ToLower(env) == "true" || env == "1"
	}

	// Fall back to config file value, default to true if not set
	if viper.IsSet(keyExperimentalEnabled) {
		return viper.GetBool(keyExperimentalEnabled)
	}
	return true
}

// ExperimentalRepo returns the experimental repository from config or environment variable.
// Environment variable OCI_SYNC_EXPERIMENTAL_REPO takes precedence over config file.
func ExperimentalRepo() (string, error) {
	// Check environment variable first (takes precedence)
	envRepo := strings.TrimSpace(os.Getenv("OCI_SYNC_EXPERIMENTAL_REPO"))
	if envRepo != "" {
		return validateRepo(envRepo)
	}

	// Fall back to config file value
	repo := strings.TrimSpace(viper.GetString(keyExperimentalRepo))
	if repo == "" {
		return "", fmt.Errorf("experimental repository not configured: set %s environment variable or experimental.repo in config file", "OCI_SYNC_EXPERIMENTAL_REPO")
	}

	return validateRepo(repo)
}

func validateRepo(repo string) (string, error) {
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

// InitConfig initializes the Viper configuration.
// It sets up config file search paths and environment variable prefixes.
func InitConfig() {
	// Set environment variable prefix
	viper.SetEnvPrefix("OCI_SYNC")

	// Enable automatic env variable mapping (OCI_SYNC_EXPERIMENTAL_REPO -> experimental.repo)
	viper.AutomaticEnv()

	// Set default config file name
	viper.SetConfigName(ConfigFileName)
	viper.SetConfigType(ConfigFileType)

	// Add config file search paths
	// 1. Current working directory
	viper.AddConfigPath(".")
	// 2. User home directory
	viper.AddConfigPath("$HOME/.config/oci-sync")
	// 3. Application data directory (platform-specific)
	// Note: We'll let viper handle this automatically via config directory

	// Attempt to read config file (don't fail if not found)
	if err := viper.ReadInConfig(); err != nil {
		// Only log debug message, not an error - config file is optional
		_ = err
	}
}

// ConfigFileUsed returns the path of the config file that was used.
// Returns empty string if no config file was used.
func ConfigFileUsed() string {
	if viper.ConfigFileUsed() != "" {
		return viper.ConfigFileUsed()
	}
	return ""
}
