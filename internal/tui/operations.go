package tui

import (
	"fmt"
	"os"
	"path/filepath"

	"charm.land/log/v2"
	"github.com/tiramission/oci-sync/internal/archive"
	"github.com/tiramission/oci-sync/internal/config"
	"github.com/tiramission/oci-sync/internal/crypto"
	"github.com/tiramission/oci-sync/internal/oci"
)

// PushArtifact performs a push operation
func (m *Model) PushArtifact(localPath, tag, passphrase string) error {
	if m.selectedShortcut >= len(m.shortcuts) {
		return fmt.Errorf("invalid shortcut")
	}

	shortcut := m.shortcuts[m.selectedShortcut]
	repo, err := config.GetShortcutRepo(shortcut.Name)
	if err != nil {
		return err
	}

	// Validate local path
	if _, err := os.Stat(localPath); err != nil {
		return fmt.Errorf("invalid local path: %w", err)
	}

	// Pack the local path
	log.Info("Packing artifact...", "path", localPath)
	data, err := archive.Pack(localPath)
	if err != nil {
		return fmt.Errorf("pack failed: %w", err)
	}

	// Optionally encrypt
	encrypted := passphrase != ""
	if encrypted {
		log.Info("Encrypting artifact...")
		data, err = crypto.Encrypt(data, passphrase)
		if err != nil {
			return fmt.Errorf("encrypt failed: %w", err)
		}
	}

	// Push to OCI
	remoteRef := repo + ":" + tag
	log.Info("Pushing to registry...", "ref", remoteRef)
	if err := oci.Push(m.ctx, data, remoteRef, encrypted, nil); err != nil {
		return fmt.Errorf("push failed: %w", err)
	}

	log.Info("Push successful", "ref", remoteRef)
	return nil
}

// PullArtifact performs a pull operation
func (m *Model) PullArtifact(downloadPath, passphrase string) error {
	if m.selectedShortcut >= len(m.shortcuts) || m.selectedArtifact >= len(m.artifacts) {
		return fmt.Errorf("invalid selection")
	}

	shortcut := m.shortcuts[m.selectedShortcut]
	artifact := m.artifacts[m.selectedArtifact]

	repo, err := config.GetShortcutRepo(shortcut.Name)
	if err != nil {
		return err
	}

	remoteRef := repo + ":" + artifact.Tag
	log.Info("Checking encryption status...", "ref", remoteRef)
	encrypted, err := oci.IsEncrypted(m.ctx, remoteRef)
	if err != nil {
		return fmt.Errorf("check encrypted failed: %w", err)
	}

	if encrypted && passphrase == "" {
		return fmt.Errorf("artifact is encrypted but no passphrase provided")
	}

	log.Info("Pulling from registry...", "ref", remoteRef)
	result, err := oci.Pull(m.ctx, remoteRef)
	if err != nil {
		return fmt.Errorf("pull failed: %w", err)
	}

	data := result.Data

	// Decrypt if needed
	if encrypted {
		log.Info("Decrypting artifact...")
		data, err = crypto.Decrypt(data, passphrase)
		if err != nil {
			return fmt.Errorf("decrypt failed: %w", err)
		}
	}

	// Ensure target directory exists
	targetDir := filepath.Dir(downloadPath)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("create directory failed: %w", err)
	}

	// Unpack to local path
	log.Info("Unpacking artifact...", "path", downloadPath)
	if err := archive.Unpack(data, downloadPath); err != nil {
		return fmt.Errorf("unpack failed: %w", err)
	}

	log.Info("Pull successful", "path", downloadPath)
	return nil
}

// DeleteArtifact performs a delete operation
func (m *Model) DeleteArtifact() error {
	if m.selectedShortcut >= len(m.shortcuts) || m.selectedArtifact >= len(m.artifacts) {
		return fmt.Errorf("invalid selection")
	}

	shortcut := m.shortcuts[m.selectedShortcut]
	artifact := m.artifacts[m.selectedArtifact]

	repo, err := config.GetShortcutRepo(shortcut.Name)
	if err != nil {
		return err
	}

	remoteRef := repo + ":" + artifact.Tag
	log.Info("Deleting artifact...", "ref", remoteRef)
	if err := oci.Delete(m.ctx, remoteRef); err != nil {
		return fmt.Errorf("delete failed: %w", err)
	}

	log.Info("Delete successful", "ref", remoteRef)
	return nil
}
