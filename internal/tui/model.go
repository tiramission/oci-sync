package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tiramission/oci-sync/internal/config"
	"github.com/tiramission/oci-sync/internal/oci"
)

// Screen represents different screens in the TUI
type Screen string

const (
	ScreenShortcuts Screen = "shortcuts"
	ScreenArtifacts Screen = "artifacts"
	ScreenDetail    Screen = "detail"
)

// Model is the main TUI model
type Model struct {
	ctx      context.Context
	width    int
	height   int
	screen   Screen
	quitting bool

	// Data
	shortcuts []config.ShortcutInfo
	artifacts []oci.ArtifactInfo

	// Navigation
	selectedShortcut int
	selectedArtifact int

	// Loading & errors
	loading    bool
	loadingMsg string
	err        error
	errMsg     string

	// Modal state
	modal *Modal
}

// New creates a new TUI model
func New(ctx context.Context) *Model {
	return &Model{
		ctx:              ctx,
		screen:           ScreenShortcuts,
		selectedShortcut: 0,
		selectedArtifact: 0,
	}
}

// Init initializes the model
func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		m.loadShortcuts(),
	)
}

// Update handles messages
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		// Handle quit
		if msg.String() == "ctrl+c" {
			m.quitting = true
			return m, tea.Quit
		}

		// Handle modal first
		if m.modal != nil {
			if m.modal.Type == ModalUpload {
				keep := m.modal.HandleInput(msg)
				if !keep {
					m.modal = nil
				} else if m.modal.Upload.step > 2 {
					// Submit upload
					m.loading = true
					m.loadingMsg = "Uploading artifact..."
					m.modal = nil
					return m, m.performUpload()
				}
				return m, nil
			} else if m.modal.Type == ModalDownload {
				keep := m.modal.HandleInput(msg)
				if !keep {
					m.modal = nil
				} else if m.modal.Download.step > 1 {
					// Submit download
					m.loading = true
					m.loadingMsg = "Downloading artifact..."
					m.modal = nil
					return m, m.performDownload()
				}
				return m, nil
			} else if m.modal.Type == ModalDelete {
				if msg.String() == "y" {
					m.loading = true
					m.loadingMsg = "Deleting artifact..."
					m.modal = nil
					return m, m.performDelete()
				} else if msg.String() == "n" || msg.String() == "esc" {
					m.modal = nil
				}
				return m, nil
			}
		}

		// Global quit
		if msg.String() == "q" {
			m.quitting = true
			return m, tea.Quit
		}

		// Handle screen-specific keys
		switch m.screen {
		case ScreenShortcuts:
			return m.handleShortcutsKey(msg)
		case ScreenArtifacts:
			return m.handleArtifactsKey(msg)
		case ScreenDetail:
			return m.handleDetailKey(msg)
		}

	case ShortcutsLoadedMsg:
		m.shortcuts = msg.shortcuts
		m.loading = false
		return m, nil

	case ArtifactsLoadedMsg:
		m.artifacts = msg.artifacts
		m.loading = false
		m.selectedArtifact = 0
		return m, nil

	case ErrorMsg:
		m.err = msg.err
		m.errMsg = msg.msg
		m.loading = false
		return m, nil

	case SuccessMsg:
		m.errMsg = fmt.Sprintf("✓ %s", msg.msg)
		m.loading = false
		return m, nil
	}

	return m, nil
}

// View renders the UI
func (m *Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	if m.quitting {
		return ""
	}

	// Check if showing modal
	if m.modal != nil {
		mainView := m.renderFullScreen()
		modalView := m.modal.Render(m.width, m.height)
		return lipgloss.JoinVertical(
			lipgloss.Center,
			mainView,
			"\n",
			modalView,
		)
	}

	return m.renderFullScreen()
}

// Key handlers

func (m *Model) handleShortcutsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.selectedShortcut > 0 {
			m.selectedShortcut--
		}
	case "down", "j":
		if m.selectedShortcut < len(m.shortcuts)-1 {
			m.selectedShortcut++
		}
	case "enter":
		m.screen = ScreenArtifacts
		m.loading = true
		m.loadingMsg = "Loading artifacts..."
		return m, m.loadArtifacts()
	}

	return m, nil
}

func (m *Model) handleArtifactsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.selectedArtifact > 0 {
			m.selectedArtifact--
		}
	case "down", "j":
		if m.selectedArtifact < len(m.artifacts)-1 {
			m.selectedArtifact++
		}
	case "enter":
		m.screen = ScreenDetail
	case "b":
		m.screen = ScreenShortcuts
		m.selectedArtifact = 0
	case "u":
		// Upload
		m.modal = NewUploadModal()
	case "d":
		// Download
		m.modal = NewDownloadModal()
	case "x":
		// Delete
		m.modal = NewDeleteModal()
	case "r":
		m.loading = true
		m.loadingMsg = "Refreshing artifacts..."
		return m, m.loadArtifacts()
	}

	return m, nil
}

func (m *Model) handleDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "b":
		m.screen = ScreenArtifacts
	}

	return m, nil
}

// Commands

func (m *Model) performUpload() tea.Cmd {
	return func() tea.Msg {
		if m.modal == nil || m.modal.Upload == nil {
			return ErrorMsg{msg: "Invalid modal state", err: fmt.Errorf("no upload modal")}
		}

		err := m.PushArtifact(m.modal.Upload.localPath, m.modal.Upload.tag, m.modal.Upload.passphrase)
		if err != nil {
			return ErrorMsg{msg: fmt.Sprintf("Upload failed: %v", err), err: err}
		}

		// Reload artifacts
		return m.loadArtifactsForRefresh()
	}
}

func (m *Model) performDownload() tea.Cmd {
	return func() tea.Msg {
		if m.modal == nil || m.modal.Download == nil {
			return ErrorMsg{msg: "Invalid modal state", err: fmt.Errorf("no download modal")}
		}

		err := m.PullArtifact(m.modal.Download.targetPath, m.modal.Download.passphrase)
		if err != nil {
			return ErrorMsg{msg: fmt.Sprintf("Download failed: %v", err), err: err}
		}

		return SuccessMsg{msg: "Download successful"}
	}
}

func (m *Model) performDelete() tea.Cmd {
	return func() tea.Msg {
		err := m.DeleteArtifact()
		if err != nil {
			return ErrorMsg{msg: fmt.Sprintf("Delete failed: %v", err), err: err}
		}

		// Reload artifacts
		return m.loadArtifactsForRefresh()
	}
}

func (m *Model) loadArtifactsForRefresh() tea.Msg {
	if m.selectedShortcut >= len(m.shortcuts) {
		return ErrorMsg{msg: "Invalid shortcut", err: fmt.Errorf("index out of range")}
	}

	shortcut := m.shortcuts[m.selectedShortcut]
	repo, err := config.GetShortcutRepo(shortcut.Name)
	if err != nil {
		return ErrorMsg{msg: fmt.Sprintf("Failed to get repo: %v", err), err: err}
	}

	artifacts, err := oci.List(m.ctx, repo)
	if err != nil {
		return ErrorMsg{msg: fmt.Sprintf("Failed to load artifacts: %v", err), err: err}
	}

	return ArtifactsLoadedMsg{artifacts: artifacts}
}

func (m *Model) loadShortcuts() tea.Cmd {
	return func() tea.Msg {
		shortcuts := config.GetAllShortcuts()
		return ShortcutsLoadedMsg{shortcuts: shortcuts}
	}
}

func (m *Model) loadArtifacts() tea.Cmd {
	return func() tea.Msg {
		return m.loadArtifactsForRefresh()
	}
}

// Message types

// Message types

type ShortcutsLoadedMsg struct {
	shortcuts []config.ShortcutInfo
}

type ArtifactsLoadedMsg struct {
	artifacts []oci.ArtifactInfo
}

type ErrorMsg struct {
	msg string
	err error
}

type SuccessMsg struct {
	msg string
}
