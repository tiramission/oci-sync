package tui

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tiramission/oci-sync/internal/config"
)

func TestNewModel(t *testing.T) {
	ctx := context.Background()
	m := New(ctx)

	assert.NotNil(t, m)
	assert.Equal(t, ctx, m.ctx)
	assert.Equal(t, ScreenShortcuts, m.screen)
	assert.False(t, m.quitting)
	assert.Nil(t, m.modal)
}

func TestNewUploadModal(t *testing.T) {
	modal := NewUploadModal()

	assert.NotNil(t, modal)
	assert.Equal(t, ModalUpload, modal.Type)
	assert.NotNil(t, modal.Upload)
	assert.Equal(t, 0, modal.Upload.step)
}

func TestNewDownloadModal(t *testing.T) {
	modal := NewDownloadModal()

	assert.NotNil(t, modal)
	assert.Equal(t, ModalDownload, modal.Type)
	assert.NotNil(t, modal.Download)
	assert.Equal(t, 0, modal.Download.step)
}

func TestNewDeleteModal(t *testing.T) {
	modal := NewDeleteModal()

	assert.NotNil(t, modal)
	assert.Equal(t, ModalDelete, modal.Type)
	assert.NotNil(t, modal.Delete)
}

func TestLoadShortcutsInit(t *testing.T) {
	config.InitConfig()

	ctx := context.Background()
	model := New(ctx)

	shortcuts := config.GetAllShortcuts()
	assert.NotNil(t, shortcuts)
	assert.NotNil(t, model)
}
