package gui

import (
	"context"
	"fmt"
	"log"
	"sort"

	_ "github.com/gogpu/gg/gpu"
	"github.com/gogpu/gogpu"
	"github.com/gogpu/ui/app"
	"github.com/gogpu/ui/core/button"
	"github.com/gogpu/ui/core/dialog"
	"github.com/gogpu/ui/core/listview"
	"github.com/gogpu/ui/core/splitview"
	"github.com/gogpu/ui/core/textfield"
	"github.com/gogpu/ui/desktop"
	"github.com/gogpu/ui/primitives"
	"github.com/gogpu/ui/state"
	"github.com/gogpu/ui/theme/material3"
	"github.com/gogpu/ui/widget"

	"github.com/tiramission/oci-sync/internal/config"
	"github.com/tiramission/oci-sync/internal/oci"
)

type guiState struct {
	ctx context.Context

	shortcuts        []config.ShortcutInfo
	artifacts        []oci.ArtifactInfo
	selectedShortcut int
	selectedArtifact int

	m3 *material3.Theme

	// UI references for dynamic updates
	shortcutList *listview.Widget
	artifactList *listview.Widget
	detailBox    *primitives.BoxWidget

	// Signals for button state
	downloadDisabled state.Signal[bool]
	deleteDisabled   state.Signal[bool]

	// App reference for requesting redraws
	gogpuApp *gogpu.App
	uiApp    *app.App
}

func Run(ctx context.Context) error {
	m3 := material3.New(widget.Hex(0x6750A4))

	state := &guiState{
		ctx:              ctx,
		m3:               m3,
		selectedShortcut: -1,
		selectedArtifact: -1,
		downloadDisabled: state.NewSignal(true),
		deleteDisabled:   state.NewSignal(true),
	}

	gogpuApp := gogpu.NewApp(gogpu.DefaultConfig().
		WithTitle("OCI-Sync Artifact Manager").
		WithSize(1000, 700).
		WithContinuousRender(false))

	state.gogpuApp = gogpuApp

	uiApp := app.New(
		app.WithWindowProvider(gogpuApp),
		app.WithPlatformProvider(gogpuApp),
		app.WithEventSource(gogpuApp.EventSource()),
		app.WithTheme(m3.AsTheme()),
	)

	state.uiApp = uiApp

	root := state.buildUI()
	uiApp.SetRoot(root)

	if err := desktop.Run(gogpuApp, uiApp); err != nil {
		return fmt.Errorf("GUI error: %w", err)
	}

	return nil
}

func (s *guiState) buildUI() *primitives.BoxWidget {
	s.shortcuts = config.GetAllShortcuts()
	sort.Slice(s.shortcuts, func(i, j int) bool {
		return s.shortcuts[i].Name < s.shortcuts[j].Name
	})

	// Left panel: shortcuts
	s.shortcutList = s.buildShortcutList()

	leftPanel := primitives.VBox(
		primitives.Text("Shortcuts").
			FontSize(18).
			Bold().
			Color(widget.RGBA8(33, 33, 33, 255)),
		primitives.Box(s.shortcutList).
			Height(500).
			Rounded(8).
			Background(widget.RGBA8(250, 250, 250, 255)).
			BorderStyle(1, widget.RGBA8(218, 218, 218, 255)),
	).Gap(8).Padding(16)

	// Right panel: artifacts
	s.artifactList = s.buildArtifactList()

	// Buttons
	uploadBtn := button.New(
		button.Text("Upload"),
		button.VariantOpt(button.Filled),
		button.PainterOpt(material3.ButtonPainter{Theme: s.m3}),
		button.OnClick(func() {
			s.showUploadDialog()
		}),
	)

	downloadBtn := button.New(
		button.Text("Download"),
		button.VariantOpt(button.Outlined),
		button.PainterOpt(material3.ButtonPainter{Theme: s.m3}),
		button.DisabledSignal(s.downloadDisabled),
		button.OnClick(func() {
			s.showDownloadDialog()
		}),
	)

	deleteBtn := button.New(
		button.Text("Delete"),
		button.VariantOpt(button.Outlined),
		button.PainterOpt(material3.ButtonPainter{Theme: s.m3}),
		button.DisabledSignal(s.deleteDisabled),
		button.OnClick(func() {
			s.showDeleteDialog()
		}),
	)

	refreshBtn := button.New(
		button.Text("Refresh"),
		button.VariantOpt(button.TextOnly),
		button.PainterOpt(material3.ButtonPainter{Theme: s.m3}),
		button.OnClick(func() {
			s.refreshArtifacts()
		}),
	)

	buttonRow := primitives.HBox(
		uploadBtn,
		downloadBtn,
		deleteBtn,
		refreshBtn,
	).Gap(8)

	rightPanel := primitives.VBox(
		primitives.Text("Artifacts").
			FontSize(18).
			Bold().
			Color(widget.RGBA8(33, 33, 33, 255)),
		primitives.Box(s.artifactList).
			Height(400).
			Rounded(8).
			Background(widget.RGBA8(250, 250, 250, 255)).
			BorderStyle(1, widget.RGBA8(218, 218, 218, 255)),
		buttonRow,
	).Gap(8).Padding(16)

	// Detail panel
	s.detailBox = s.buildDetailPanel()

	detailPanel := primitives.VBox(
		primitives.Text("Details").
			FontSize(18).
			Bold().
			Color(widget.RGBA8(33, 33, 33, 255)),
		primitives.Box(s.detailBox).
			Height(200).
			Rounded(8).
			Background(widget.RGBA8(250, 250, 250, 255)).
			BorderStyle(1, widget.RGBA8(218, 218, 218, 255)),
	).Gap(8).Padding(16)

	// Main split view
	mainContent := splitview.New(
		splitview.First(leftPanel),
		splitview.Second(rightPanel),
		splitview.InitialRatio(0.3),
		splitview.PainterOpt(material3.SplitViewPainter{Theme: s.m3}),
	)

	// Root layout
	return primitives.VBox(
		primitives.Box(mainContent).
			Height(550).
			Rounded(12).
			Background(widget.RGBA8(255, 255, 255, 255)).
			ShadowLevel(2),
		detailPanel,
	).Padding(16).Gap(8).Background(widget.RGBA8(245, 245, 245, 255))
}

func (s *guiState) buildShortcutList() *listview.Widget {
	items := s.shortcuts

	return listview.New(
		listview.ItemCount(len(items)),
		listview.FixedItemHeight(48),
		listview.SelectionModeOpt(listview.SelectionSingle),
		listview.PainterOpt(material3.ListViewPainter{Theme: s.m3}),
		listview.BuildItem(func(ctx listview.ItemContext) widget.Widget {
			sc := items[ctx.Index]
			nameColor := widget.RGBA8(33, 33, 33, 255)
			if ctx.Selected {
				nameColor = widget.RGBA8(103, 80, 164, 255)
			}

			return primitives.VBox(
				primitives.Text(sc.Name).
					FontSize(14).
					Bold().
					Color(nameColor),
				primitives.Text(sc.Repo).
					FontSize(11).
					Color(widget.RGBA8(120, 120, 120, 255)),
			).PaddingXY(12, 6).Gap(2)
		}),
		listview.Divider(true),
		listview.OnItemClick(func(index int) {
			s.selectShortcut(index)
		}),
	)
}

func (s *guiState) buildArtifactList() *listview.Widget {
	items := s.artifacts

	return listview.New(
		listview.ItemCount(len(items)),
		listview.FixedItemHeight(48),
		listview.SelectionModeOpt(listview.SelectionSingle),
		listview.PainterOpt(material3.ListViewPainter{Theme: s.m3}),
		listview.BuildItem(func(ctx listview.ItemContext) widget.Widget {
			art := items[ctx.Index]
			tagColor := widget.RGBA8(33, 33, 33, 255)
			if ctx.Selected {
				tagColor = widget.RGBA8(103, 80, 164, 255)
			}

			// Status indicator
			status := "[ ]"
			if art.Encrypted {
				status = "[E]"
			}

			return primitives.VBox(
				primitives.HBox(
					primitives.Text(status+" "+art.Tag).
						FontSize(14).
						Bold().
						Color(tagColor),
					primitives.Text(formatSize(art.Size)).
						FontSize(11).
						Color(widget.RGBA8(120, 120, 120, 255)),
				).Gap(8),
				primitives.Text(art.Version).
					FontSize(11).
					Color(widget.RGBA8(150, 150, 150, 255)),
			).PaddingXY(12, 6).Gap(2)
		}),
		listview.Divider(true),
		listview.OnItemClick(func(index int) {
			s.selectArtifact(index)
		}),
	)
}

func (s *guiState) buildDetailPanel() *primitives.BoxWidget {
	if s.selectedArtifact < 0 || s.selectedArtifact >= len(s.artifacts) {
		return primitives.Box(
			primitives.Text("Select an artifact to view details").
				FontSize(14).
				Color(widget.RGBA8(150, 150, 150, 255)),
		).Padding(16)
	}

	art := s.artifacts[s.selectedArtifact]

	return primitives.VBox(
		primitives.HBox(
			primitives.Text("Tag:").FontSize(13).Bold().Color(widget.RGBA8(66, 66, 66, 255)),
			primitives.Text(art.Tag).FontSize(13).Color(widget.RGBA8(33, 33, 33, 255)),
		).Gap(8),
		primitives.HBox(
			primitives.Text("Repository:").FontSize(13).Bold().Color(widget.RGBA8(66, 66, 66, 255)),
			primitives.Text(art.Repo).FontSize(13).Color(widget.RGBA8(33, 33, 33, 255)),
		).Gap(8),
		primitives.HBox(
			primitives.Text("Digest:").FontSize(13).Bold().Color(widget.RGBA8(66, 66, 66, 255)),
			primitives.Text(art.Digest).FontSize(13).Color(widget.RGBA8(33, 33, 33, 255)),
		).Gap(8),
		primitives.HBox(
			primitives.Text("Encrypted:").FontSize(13).Bold().Color(widget.RGBA8(66, 66, 66, 255)),
			primitives.Text(boolToString(art.Encrypted)).FontSize(13).Color(widget.RGBA8(33, 33, 33, 255)),
		).Gap(8),
		primitives.HBox(
			primitives.Text("Version:").FontSize(13).Bold().Color(widget.RGBA8(66, 66, 66, 255)),
			primitives.Text(art.Version).FontSize(13).Color(widget.RGBA8(33, 33, 33, 255)),
		).Gap(8),
	).Padding(16).Gap(4)
}

func (s *guiState) selectShortcut(index int) {
	if index < 0 || index >= len(s.shortcuts) {
		return
	}

	s.selectedShortcut = index
	s.selectedArtifact = -1
	s.artifacts = nil

	// Disable action buttons
	s.downloadDisabled.Set(true)
	s.deleteDisabled.Set(true)

	// Load artifacts in background
	go func() {
		shortcut := s.shortcuts[index]
		repo, err := config.GetShortcutRepo(shortcut.Name)
		if err != nil {
			log.Printf("Error getting repo: %v", err)
			return
		}

		artifacts, err := oci.List(s.ctx, repo)
		if err != nil {
			log.Printf("Error listing artifacts: %v", err)
			return
		}

		s.artifacts = artifacts
		s.artifactList = s.buildArtifactList()
		s.detailBox = s.buildDetailPanel()
		s.gogpuApp.RequestRedraw()
	}()
}

func (s *guiState) selectArtifact(index int) {
	if index < 0 || index >= len(s.artifacts) {
		return
	}

	s.selectedArtifact = index
	s.detailBox = s.buildDetailPanel()

	// Enable action buttons
	s.downloadDisabled.Set(false)
	s.deleteDisabled.Set(false)

	s.gogpuApp.RequestRedraw()
}

func (s *guiState) refreshArtifacts() {
	if s.selectedShortcut < 0 || s.selectedShortcut >= len(s.shortcuts) {
		return
	}

	s.selectShortcut(s.selectedShortcut)
}

func (s *guiState) showUploadDialog() {
	if s.selectedShortcut < 0 || s.selectedShortcut >= len(s.shortcuts) {
		return
	}

	localPathField := textfield.New(
		textfield.Placeholder("Local file or directory path"),
		textfield.PainterOpt(material3.TextFieldPainter{Theme: s.m3}),
	)

	tagField := textfield.New(
		textfield.Placeholder("Tag (e.g., v1.0, latest)"),
		textfield.PainterOpt(material3.TextFieldPainter{Theme: s.m3}),
	)

	passphraseField := textfield.New(
		textfield.Placeholder("Passphrase (optional, for encryption)"),
		textfield.InputTypeOpt(textfield.TypePassword),
		textfield.PainterOpt(material3.TextFieldPainter{Theme: s.m3}),
	)

	content := primitives.VBox(
		primitives.Text("Upload Artifact").
			FontSize(18).
			Bold().
			Color(widget.RGBA8(33, 33, 33, 255)),
		localPathField,
		tagField,
		passphraseField,
	).Gap(12)

	dlg := dialog.New(
		dialog.Title("Upload"),
		dialog.Content(content),
		dialog.Actions(
			dialog.Action{
				Label: "Cancel",
				OnClick: func() {
					// Dialog closes automatically
				},
			},
			dialog.Action{
				Label: "Upload",
				Default: true,
				OnClick: func() {
					localPath := localPathField.Text()
					tag := tagField.Text()
					passphrase := passphraseField.Text()

					if localPath == "" || tag == "" {
						return
					}

					go func() {
						err := s.pushArtifact(localPath, tag, passphrase)
						if err != nil {
							log.Printf("Upload failed: %v", err)
						} else {
							s.refreshArtifacts()
						}
					}()
				},
			},
		),
		dialog.MaxWidth(400),
		dialog.PainterOpt(material3.DialogPainter{Theme: s.m3}),
	)

	// Show dialog via window context
	s.uiApp.Window().Context().OverlayManager().PushOverlay(dlg, nil)
	s.gogpuApp.RequestRedraw()
}

func (s *guiState) showDownloadDialog() {
	if s.selectedShortcut < 0 || s.selectedArtifact < 0 {
		return
	}

	shortcut := s.shortcuts[s.selectedShortcut]
	artifact := s.artifacts[s.selectedArtifact]

	localPathField := textfield.New(
		textfield.Placeholder("Download to path"),
		textfield.PainterOpt(material3.TextFieldPainter{Theme: s.m3}),
	)

	passphraseField := textfield.New(
		textfield.Placeholder("Passphrase (if encrypted)"),
		textfield.InputTypeOpt(textfield.TypePassword),
		textfield.PainterOpt(material3.TextFieldPainter{Theme: s.m3}),
	)

	content := primitives.VBox(
		primitives.Text("Download Artifact").
			FontSize(18).
			Bold().
			Color(widget.RGBA8(33, 33, 33, 255)),
		primitives.Text(fmt.Sprintf("Downloading: %s:%s", shortcut.Name, artifact.Tag)).
			FontSize(14).
			Color(widget.RGBA8(100, 100, 100, 255)),
		localPathField,
		passphraseField,
	).Gap(12)

	dlg := dialog.New(
		dialog.Title("Download"),
		dialog.Content(content),
		dialog.Actions(
			dialog.Action{
				Label: "Cancel",
				OnClick: func() {
					// Dialog closes automatically
				},
			},
			dialog.Action{
				Label: "Download",
				Default: true,
				OnClick: func() {
					localPath := localPathField.Text()
					passphrase := passphraseField.Text()

					if localPath == "" {
						return
					}

					go func() {
						err := s.pullArtifact(localPath, passphrase)
						if err != nil {
							log.Printf("Download failed: %v", err)
						}
					}()
				},
			},
		),
		dialog.MaxWidth(400),
		dialog.PainterOpt(material3.DialogPainter{Theme: s.m3}),
	)

	// Show dialog via window context
	s.uiApp.Window().Context().OverlayManager().PushOverlay(dlg, nil)
	s.gogpuApp.RequestRedraw()
}

func (s *guiState) showDeleteDialog() {
	if s.selectedShortcut < 0 || s.selectedArtifact < 0 {
		return
	}

	shortcut := s.shortcuts[s.selectedShortcut]
	artifact := s.artifacts[s.selectedArtifact]

	content := primitives.VBox(
		primitives.Text("Delete Artifact").
			FontSize(18).
			Bold().
			Color(widget.RGBA8(33, 33, 33, 255)),
		primitives.Text(fmt.Sprintf("Are you sure you want to delete %s:%s?", shortcut.Name, artifact.Tag)).
			FontSize(14).
			Color(widget.RGBA8(100, 100, 100, 255)),
	).Gap(12)

	dlg := dialog.New(
		dialog.Title("Confirm Delete"),
		dialog.Content(content),
		dialog.Actions(
			dialog.Action{
				Label: "Cancel",
				OnClick: func() {
					// Dialog closes automatically
				},
			},
			dialog.Action{
				Label: "Delete",
				Default: true,
				OnClick: func() {
					go func() {
						err := s.deleteArtifact()
						if err != nil {
							log.Printf("Delete failed: %v", err)
						} else {
							s.refreshArtifacts()
						}
					}()
				},
			},
		),
		dialog.MaxWidth(400),
		dialog.PainterOpt(material3.DialogPainter{Theme: s.m3}),
	)

	// Show dialog via window context
	s.uiApp.Window().Context().OverlayManager().PushOverlay(dlg, nil)
	s.gogpuApp.RequestRedraw()
}

// Helper functions

func formatSize(bytes int64) string {
	units := []string{"B", "KB", "MB", "GB", "TB"}
	size := float64(bytes)
	unitIdx := 0

	for size >= 1024 && unitIdx < len(units)-1 {
		size /= 1024
		unitIdx++
	}

	if unitIdx == 0 {
		return fmt.Sprintf("%dB", int64(size))
	}
	return fmt.Sprintf("%.1f%s", size, units[unitIdx])
}

func boolToString(b bool) string {
	if b {
		return "Yes"
	}
	return "No"
}
