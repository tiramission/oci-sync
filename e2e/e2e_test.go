package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	// Test registry - should be configured by CI or test environment
	testRegistry   = "internal.183867412.xyz:5000"
	testRepoPrefix = "oci-sync-e2e"
)

// TestMain handles setup and teardown for all e2e tests.
func TestMain(m *testing.M) {
	// Build the binary before running tests
	err := buildBinary()
	if err != nil {
		panic("failed to build binary: " + err.Error())
	}

	os.Exit(m.Run())
}

func buildBinary() error {
	if err := os.MkdirAll(filepath.Join(projectRoot(), "temps"), 0755); err != nil {
		return err
	}
	cmd := exec.Command("go", "build", "-o", binaryPath(), ".")
	cmd.Dir = projectRoot()
	return cmd.Run()
}

func projectRoot() string {
	// Assume e2e directory is at project root/e2e
	wd, _ := os.Getwd()
	return filepath.Dir(wd)
}

// sanitizeRepoName replaces invalid characters with dashes for repository names.
func sanitizeRepoName(name string) string {
	// Replace any non-alphanumeric characters (except dash, dot, underscore) with dash
	var result strings.Builder
	for _, c := range name {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '.' || c == '_' {
			result.WriteRune(c)
		} else {
			result.WriteRune('-')
		}
	}
	return result.String()
}

func binaryPath() string {
	return filepath.Join(projectRoot(), "temps", "oci-sync-test")
}

// setupTestDir creates a temporary directory with test files.
func setupTestDir(t *testing.T) string {
	tmpDir := t.TempDir()

	// Create test file
	testFile := filepath.Join(tmpDir, "test.txt")
	err := os.WriteFile(testFile, []byte("hello from e2e test"), 0644)
	require.NoError(t, err)

	// Create test directory with files
	testDir := filepath.Join(tmpDir, "testdir")
	err = os.MkdirAll(testDir, 0755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(testDir, "file1.txt"), []byte("content 1"), 0644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(testDir, "file2.txt"), []byte("content 2"), 0644)
	require.NoError(t, err)

	return tmpDir
}

// setupConfig creates a config file for experimental commands.
func setupConfig(t *testing.T, dir, repo string) string {
	configPath := filepath.Join(dir, "oci-sync.yaml")
	configContent := `experimental:
  enabled: true
  repo: ` + repo + `
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)
	return configPath
}

// runCmd runs the oci-sync binary with given args and returns output.
func runCmd(t *testing.T, dir string, args ...string) (string, error) {
	cmd := exec.Command(binaryPath(), args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// TestPushPull tests basic push and pull operations.
func TestPushPull(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	testDir := setupTestDir(t)
	repo := testRegistry + "/" + sanitizeRepoName(testRepoPrefix+"-pushpull-"+t.Name())
	_ = setupConfig(t, testDir, repo)

	testFile := filepath.Join(testDir, "test.txt")
	tag := "v1"

	// Push
	output, err := runCmd(t, testDir, "push", "--local", testFile, "--remote", repo+":"+tag)
	require.NoError(t, err, "push failed: %s", output)
	assert.Contains(t, output, "Push successful")

	// Pull to new directory
	pullDir := filepath.Join(testDir, "pull-output")
	err = os.MkdirAll(pullDir, 0755)
	require.NoError(t, err)

	output, err = runCmd(t, testDir, "pull", "--remote", repo+":"+tag, "--local", pullDir)
	require.NoError(t, err, "pull failed: %s", output)
	assert.Contains(t, output, "Pull successful")

	// Verify content
	pulledContent, err := os.ReadFile(filepath.Join(pullDir, "test.txt"))
	require.NoError(t, err)
	assert.Equal(t, "hello from e2e test", string(pulledContent))
}

// TestPushPullDirectory tests push and pull for directories.
func TestPushPullDirectory(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	testDir := setupTestDir(t)
	repo := testRegistry + "/" + sanitizeRepoName(testRepoPrefix+"-pushdir-"+t.Name())
	_ = setupConfig(t, testDir, repo)

	testDirPath := filepath.Join(testDir, "testdir")
	tag := "v1"

	// Push directory
	output, err := runCmd(t, testDir, "push", "--local", testDirPath, "--remote", repo+":"+tag)
	require.NoError(t, err, "push failed: %s", output)
	assert.Contains(t, output, "Push successful")

	// Pull
	pullDir := filepath.Join(testDir, "pull-output")
	err = os.MkdirAll(pullDir, 0755)
	require.NoError(t, err)

	output, err = runCmd(t, testDir, "pull", "--remote", repo+":"+tag, "--local", pullDir)
	require.NoError(t, err, "pull failed: %s", output)
	assert.Contains(t, output, "Pull successful")

	// Verify files exist
	assert.FileExists(t, filepath.Join(pullDir, "testdir", "file1.txt"))
	assert.FileExists(t, filepath.Join(pullDir, "testdir", "file2.txt"))
}

// TestPushPullEncrypted tests push and pull with encryption.
func TestPushPullEncrypted(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	testDir := setupTestDir(t)
	repo := testRegistry + "/" + sanitizeRepoName(testRepoPrefix+"-encrypt-"+t.Name())
	_ = setupConfig(t, testDir, repo)

	testFile := filepath.Join(testDir, "test.txt")
	tag := "v1"
	passphrase := "test-secret-123"

	// Push with encryption
	output, err := runCmd(t, testDir, "push", "--local", testFile, "--remote", repo+":"+tag, "--passphrase", passphrase)
	require.NoError(t, err, "push failed: %s", output)
	assert.Contains(t, output, "Push successful")

	// Pull with wrong passphrase should fail
	pullDir := filepath.Join(testDir, "pull-wrong")
	err = os.MkdirAll(pullDir, 0755)
	require.NoError(t, err)

	_, err = runCmd(t, testDir, "pull", "--remote", repo+":"+tag, "--local", pullDir, "--passphrase", "wrong-passphrase")
	assert.Error(t, err, "pull with wrong passphrase should fail")

	// Pull with correct passphrase
	pullDir = filepath.Join(testDir, "pull-correct")
	err = os.MkdirAll(pullDir, 0755)
	require.NoError(t, err)

	output, err = runCmd(t, testDir, "pull", "--remote", repo+":"+tag, "--local", pullDir, "--passphrase", passphrase)
	require.NoError(t, err, "pull failed: %s", output)
	assert.Contains(t, output, "Pull successful")

	// Verify content
	pulledContent, err := os.ReadFile(filepath.Join(pullDir, "test.txt"))
	require.NoError(t, err)
	assert.Equal(t, "hello from e2e test", string(pulledContent))
}

// TestXPushPull tests experimental x push/pull commands.
func TestXPushPull(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	testDir := setupTestDir(t)
	repo := testRegistry + "/" + sanitizeRepoName(testRepoPrefix+"-x-"+t.Name())
	_ = setupConfig(t, testDir, repo)

	testFile := filepath.Join(testDir, "test.txt")
	tag := "v1"

	// x push
	output, err := runCmd(t, testDir, "x", "push", "--local", testFile, "--tag", tag)
	require.NoError(t, err, "x push failed: %s", output)
	assert.Contains(t, output, "Push successful")

	// x pull
	pullDir := filepath.Join(testDir, "pull-output")
	err = os.MkdirAll(pullDir, 0755)
	require.NoError(t, err)

	output, err = runCmd(t, testDir, "x", "pull", "--tag", tag, "--local", pullDir)
	require.NoError(t, err, "x pull failed: %s", output)
	assert.Contains(t, output, "Pull successful")

	// Verify content
	pulledContent, err := os.ReadFile(filepath.Join(pullDir, "test.txt"))
	require.NoError(t, err)
	assert.Equal(t, "hello from e2e test", string(pulledContent))
}

// TestXList tests experimental x list command.
func TestXList(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	testDir := setupTestDir(t)
	repo := testRegistry + "/" + sanitizeRepoName(testRepoPrefix+"-list-"+t.Name())
	_ = setupConfig(t, testDir, repo)

	testFile := filepath.Join(testDir, "test.txt")

	// Push a few tags
	for _, tag := range []string{"v1", "v2", "v3"} {
		output, err := runCmd(t, testDir, "x", "push", "--local", testFile, "--tag", tag)
		require.NoError(t, err, "push failed for tag %s: %s", tag, output)
	}

	// x list
	output, err := runCmd(t, testDir, "x", "list")
	require.NoError(t, err, "x list failed: %s", output)
	assert.Contains(t, output, "v1")
	assert.Contains(t, output, "v2")
	assert.Contains(t, output, "v3")
}

// TestXDelete tests experimental x delete command.
func TestXDelete(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	testDir := setupTestDir(t)
	repo := testRegistry + "/" + sanitizeRepoName(testRepoPrefix+"-delete-"+t.Name())
	_ = setupConfig(t, testDir, repo)

	testFile := filepath.Join(testDir, "test.txt")
	tag := "v1"

	// Push
	output, err := runCmd(t, testDir, "x", "push", "--local", testFile, "--tag", tag)
	require.NoError(t, err, "push failed: %s", output)

	// x delete
	output, err = runCmd(t, testDir, "x", "delete", "--tag", tag)
	require.NoError(t, err, "x delete failed: %s", output)
	assert.Contains(t, output, "Delete successful")

	// Verify deleted - list should not contain the tag
	output, err = runCmd(t, testDir, "x", "list")
	require.NoError(t, err, "x list failed: %s", output)
	assert.NotContains(t, output, tag)
}

// TestConfigFile tests config file support.
func TestConfigFile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	testDir := setupTestDir(t)
	repo := testRegistry + "/" + sanitizeRepoName(testRepoPrefix+"-config-"+t.Name())
	setupConfig(t, testDir, repo)

	testFile := filepath.Join(testDir, "test.txt")
	tag := "v1"

	// Use config file without explicit repo
	output, err := runCmd(t, testDir, "x", "push", "--local", testFile, "--tag", tag)
	require.NoError(t, err, "push with config failed: %s", output)
	assert.Contains(t, output, "Push successful")
}

// TestConfigDisabled tests experimental disabled in config.
func TestConfigDisabled(t *testing.T) {
	testDir := setupTestDir(t)

	// Create config with disabled experimental
	configPath := filepath.Join(testDir, "oci-sync.yaml")
	err := os.WriteFile(configPath, []byte(`experimental:
  enabled: false
`), 0644)
	require.NoError(t, err)

	// x command should not be available
	output, err := runCmd(t, testDir, "x", "list")
	assert.Error(t, err)
	assert.Contains(t, output, "unknown command")
}

// TestEnvVarOverrideConfig tests environment variable override.
func TestEnvVarOverrideConfig(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	testDir := setupTestDir(t)
	repo := testRegistry + "/" + sanitizeRepoName(testRepoPrefix+"-env-"+t.Name())

	// Set config with different repo
	setupConfig(t, testDir, "config-repo.example.com/repo")

	// But use env var to override
	testFile := filepath.Join(testDir, "test.txt")
	tag := "v1"

	// Set env var
	t.Setenv("OCI_SYNC_EXPERIMENTAL_REPO", repo)

	// Push should use env var repo
	output, err := runCmd(t, testDir, "x", "push", "--local", testFile, "--tag", tag)
	require.NoError(t, err, "push with env override failed: %s", output)
	assert.Contains(t, output, repo)
}
