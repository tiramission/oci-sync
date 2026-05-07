package gui

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

func (s *guiState) pushArtifact(localPath, tag, passphrase string) error {
	if s.selectedShortcut < 0 || s.selectedShortcut >= len(s.shortcuts) {
		return fmt.Errorf("no shortcut selected")
	}

	shortcut := s.shortcuts[s.selectedShortcut]
	repo, err := config.GetShortcutRepo(shortcut.Name)
	if err != nil {
		return fmt.Errorf("get repo: %w", err)
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
	if err := oci.Push(s.ctx, data, remoteRef, encrypted, nil); err != nil {
		return fmt.Errorf("push failed: %w", err)
	}

	log.Info("Push successful", "ref", remoteRef)
	return nil
}

func (s *guiState) pullArtifact(downloadPath, passphrase string) error {
	if s.selectedShortcut < 0 || s.selectedShortcut >= len(s.shortcuts) {
		return fmt.Errorf("no shortcut selected")
	}

	if s.selectedArtifact < 0 || s.selectedArtifact >= len(s.artifacts) {
		return fmt.Errorf("no artifact selected")
	}

	shortcut := s.shortcuts[s.selectedShortcut]
	artifact := s.artifacts[s.selectedArtifact]

	repo, err := config.GetShortcutRepo(shortcut.Name)
	if err != nil {
		return fmt.Errorf("get repo: %w", err)
	}

	remoteRef := repo + ":" + artifact.Tag
	log.Info("Checking encryption status...", "ref", remoteRef)
	encrypted, err := oci.IsEncrypted(s.ctx, remoteRef)
	if err != nil {
		return fmt.Errorf("check encrypted failed: %w", err)
	}

	if encrypted && passphrase == "" {
		return fmt.Errorf("artifact is encrypted but no passphrase provided")
	}

	log.Info("Pulling from registry...", "ref", remoteRef)
	result, err := oci.Pull(s.ctx, remoteRef)
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

func (s *guiState) deleteArtifact() error {
	if s.selectedShortcut < 0 || s.selectedShortcut >= len(s.shortcuts) {
		return fmt.Errorf("no shortcut selected")
	}

	if s.selectedArtifact < 0 || s.selectedArtifact >= len(s.artifacts) {
		return fmt.Errorf("no artifact selected")
	}

	shortcut := s.shortcuts[s.selectedShortcut]
	artifact := s.artifacts[s.selectedArtifact]

	repo, err := config.GetShortcutRepo(shortcut.Name)
	if err != nil {
		return fmt.Errorf("get repo: %w", err)
	}

	remoteRef := repo + ":" + artifact.Tag
	log.Info("Deleting artifact...", "ref", remoteRef)
	if err := oci.Delete(s.ctx, remoteRef); err != nil {
		return fmt.Errorf("delete failed: %w", err)
	}

	log.Info("Delete successful", "ref", remoteRef)
	return nil
}
