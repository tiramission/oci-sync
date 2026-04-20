package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Modal types
type ModalType string

const (
	ModalUpload   ModalType = "upload"
	ModalDownload ModalType = "download"
	ModalDelete   ModalType = "delete"
	ModalConfirm  ModalType = "confirm"
)

// UploadModal state
type UploadModal struct {
	localPath  string
	tag        string
	passphrase string
	step       int // 0: path, 1: tag, 2: passphrase
}

// DownloadModal state
type DownloadModal struct {
	targetPath string
	passphrase string
	step       int // 0: path, 1: passphrase
}

// DeleteModal state
type DeleteModal struct {
	confirmed bool
}

// ConfirmModal state
type ConfirmModal struct {
	message   string
	confirmed bool
}

// Modal represents an active modal
type Modal struct {
	Type        ModalType
	Upload      *UploadModal
	Download    *DownloadModal
	Delete      *DeleteModal
	Confirm     *ConfirmModal
	inputBuffer string
	errorMsg    string
	isPassword  bool
}

// NewUploadModal creates a new upload modal
func NewUploadModal() *Modal {
	return &Modal{
		Type:   ModalUpload,
		Upload: &UploadModal{step: 0},
	}
}

// NewDownloadModal creates a new download modal
func NewDownloadModal() *Modal {
	return &Modal{
		Type:     ModalDownload,
		Download: &DownloadModal{step: 0},
	}
}

// NewDeleteModal creates a new delete modal
func NewDeleteModal() *Modal {
	return &Modal{
		Type:   ModalDelete,
		Delete: &DeleteModal{},
	}
}

// Render renders the modal
func (modal *Modal) Render(width, height int) string {
	switch modal.Type {
	case ModalUpload:
		return modal.renderUpload(width, height)
	case ModalDownload:
		return modal.renderDownload(width, height)
	case ModalDelete:
		return modal.renderDelete(width, height)
	case ModalConfirm:
		return modal.renderConfirm(width, height)
	}
	return ""
}

func (modal *Modal) renderUpload(width, height int) string {
	var sb strings.Builder

	title := lipgloss.NewStyle().
		Foreground(lipgloss.Color("12")).
		Bold(true).
		Padding(1, 2).
		Render("📤 Upload Artifact")
	sb.WriteString(title + "\n\n")

	// Path input
	pathLabel := lipgloss.NewStyle().
		Foreground(lipgloss.Color("11")).
		Render("Local Path:")
	sb.WriteString(pathLabel + "\n")

	if modal.Upload.step == 0 {
		// Active input
		inputStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("10")).
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("10")).
			Padding(0, 1)
		sb.WriteString(inputStyle.Render(modal.inputBuffer+"▐") + "\n\n")
	} else {
		// Completed
		pathStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("2")).
			Padding(0, 1)
		sb.WriteString(pathStyle.Render(modal.Upload.localPath) + "\n\n")
	}

	// Tag input
	tagLabel := lipgloss.NewStyle().
		Foreground(lipgloss.Color("11")).
		Render("Tag:")
	sb.WriteString(tagLabel + "\n")

	if modal.Upload.step == 1 {
		// Active input
		inputStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("10")).
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("10")).
			Padding(0, 1)
		sb.WriteString(inputStyle.Render(modal.inputBuffer+"▐") + "\n\n")
	} else if modal.Upload.step > 1 {
		// Completed
		tagStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("2")).
			Padding(0, 1)
		sb.WriteString(tagStyle.Render(modal.Upload.tag) + "\n\n")
	} else {
		// Not started
		sb.WriteString(lipgloss.NewStyle().Padding(0, 1).Render("(empty)") + "\n\n")
	}

	// Passphrase input
	passphraseLabel := lipgloss.NewStyle().
		Foreground(lipgloss.Color("11")).
		Render("Passphrase (optional):")
	sb.WriteString(passphraseLabel + "\n")

	if modal.Upload.step == 2 {
		// Active input
		inputStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("10")).
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("10")).
			Padding(0, 1)
		masked := strings.Repeat("•", len(modal.inputBuffer))
		sb.WriteString(inputStyle.Render(masked+"▐") + "\n\n")
	} else if modal.Upload.step > 2 {
		// Completed
		if modal.Upload.passphrase != "" {
			sb.WriteString(lipgloss.NewStyle().Padding(0, 1).Render("(encrypted)") + "\n\n")
		} else {
			sb.WriteString(lipgloss.NewStyle().Padding(0, 1).Render("(none)") + "\n\n")
		}
	} else {
		// Not started
		sb.WriteString(lipgloss.NewStyle().Padding(0, 1).Render("(empty)") + "\n\n")
	}

	// Error message
	if modal.errorMsg != "" {
		errorStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("1")).
			Padding(0, 1)
		sb.WriteString(errorStyle.Render(modal.errorMsg) + "\n\n")
	}

	// Help
	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")).
		Padding(0, 1)
	help := "Enter: Next | Esc: Cancel"
	if modal.Upload.step >= 2 {
		help = "Enter: Upload | Esc: Cancel"
	}
	sb.WriteString(helpStyle.Render(help))

	return sb.String()
}

func (modal *Modal) renderDownload(width, height int) string {
	var sb strings.Builder

	title := lipgloss.NewStyle().
		Foreground(lipgloss.Color("12")).
		Bold(true).
		Padding(1, 2).
		Render("⬇️  Download Artifact")
	sb.WriteString(title + "\n\n")

	// Path input
	pathLabel := lipgloss.NewStyle().
		Foreground(lipgloss.Color("11")).
		Render("Download To:")
	sb.WriteString(pathLabel + "\n")

	if modal.Download.step == 0 {
		// Active input
		inputStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("10")).
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("10")).
			Padding(0, 1)
		sb.WriteString(inputStyle.Render(modal.inputBuffer+"▐") + "\n\n")
	} else {
		// Completed
		pathStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("2")).
			Padding(0, 1)
		sb.WriteString(pathStyle.Render(modal.Download.targetPath) + "\n\n")
	}

	// Passphrase input
	passphraseLabel := lipgloss.NewStyle().
		Foreground(lipgloss.Color("11")).
		Render("Passphrase (if encrypted):")
	sb.WriteString(passphraseLabel + "\n")

	if modal.Download.step == 1 {
		// Active input
		inputStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("10")).
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("10")).
			Padding(0, 1)
		masked := strings.Repeat("•", len(modal.inputBuffer))
		sb.WriteString(inputStyle.Render(masked+"▐") + "\n\n")
	} else if modal.Download.step > 1 {
		// Completed
		if modal.Download.passphrase != "" {
			sb.WriteString(lipgloss.NewStyle().Padding(0, 1).Render("(encrypted)") + "\n\n")
		} else {
			sb.WriteString(lipgloss.NewStyle().Padding(0, 1).Render("(none)") + "\n\n")
		}
	} else {
		// Not started
		sb.WriteString(lipgloss.NewStyle().Padding(0, 1).Render("(empty)") + "\n\n")
	}

	// Error message
	if modal.errorMsg != "" {
		errorStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("1")).
			Padding(0, 1)
		sb.WriteString(errorStyle.Render(modal.errorMsg) + "\n\n")
	}

	// Help
	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")).
		Padding(0, 1)
	help := "Enter: Next | Esc: Cancel"
	if modal.Download.step >= 1 {
		help = "Enter: Download | Esc: Cancel"
	}
	sb.WriteString(helpStyle.Render(help))

	return sb.String()
}

func (modal *Modal) renderDelete(width, height int) string {
	var sb strings.Builder

	title := lipgloss.NewStyle().
		Foreground(lipgloss.Color("1")).
		Bold(true).
		Padding(1, 2).
		Render("🗑️  Delete Artifact")
	sb.WriteString(title + "\n\n")

	message := lipgloss.NewStyle().
		Foreground(lipgloss.Color("11")).
		Padding(0, 2).
		Render("Are you sure you want to delete this artifact?")
	sb.WriteString(message + "\n\n")

	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")).
		Padding(0, 2).
		Render("y: Yes | n: No")
	sb.WriteString(help)

	return sb.String()
}

func (modal *Modal) renderConfirm(width, height int) string {
	var sb strings.Builder

	title := lipgloss.NewStyle().
		Foreground(lipgloss.Color("12")).
		Bold(true).
		Padding(1, 2).
		Render("❓ Confirm")
	sb.WriteString(title + "\n\n")

	message := lipgloss.NewStyle().
		Foreground(lipgloss.Color("11")).
		Padding(0, 2).
		Render(modal.Confirm.message)
	sb.WriteString(message + "\n\n")

	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")).
		Padding(0, 2).
		Render("y: Yes | n: No")
	sb.WriteString(help)

	return sb.String()
}

// HandleInput handles input for the modal
func (modal *Modal) HandleInput(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "esc":
		return false
	case "enter":
		return modal.nextStep()
	case "y":
		if modal.Type == ModalDelete || modal.Type == ModalConfirm {
			return true
		}
	case "n":
		if modal.Type == ModalDelete || modal.Type == ModalConfirm {
			return false
		}
	case "backspace":
		if len(modal.inputBuffer) > 0 {
			modal.inputBuffer = modal.inputBuffer[:len(modal.inputBuffer)-1]
		}
	default:
		if msg.Type == tea.KeyRunes {
			modal.inputBuffer += string(msg.Runes)
		}
	}
	return true // Keep modal open
}

func (modal *Modal) nextStep() bool {
	switch modal.Type {
	case ModalUpload:
		if modal.Upload.step == 0 {
			if modal.inputBuffer == "" {
				modal.errorMsg = "Path cannot be empty"
				return true
			}
			modal.Upload.localPath = modal.inputBuffer
			modal.inputBuffer = ""
			modal.Upload.step++
		} else if modal.Upload.step == 1 {
			if modal.inputBuffer == "" {
				modal.errorMsg = "Tag cannot be empty"
				return true
			}
			modal.Upload.tag = modal.inputBuffer
			modal.inputBuffer = ""
			modal.Upload.step++
		} else if modal.Upload.step == 2 {
			modal.Upload.passphrase = modal.inputBuffer
			return true // Submit upload
		}
	case ModalDownload:
		if modal.Download.step == 0 {
			if modal.inputBuffer == "" {
				modal.errorMsg = "Path cannot be empty"
				return true
			}
			modal.Download.targetPath = modal.inputBuffer
			modal.inputBuffer = ""
			modal.Download.step++
		} else if modal.Download.step == 1 {
			modal.Download.passphrase = modal.inputBuffer
			return true // Submit download
		}
	}
	return true
}
