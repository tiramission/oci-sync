package cmd

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"charm.land/log/v2"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/tiramission/oci-sync/internal/cache"
	"github.com/tiramission/oci-sync/internal/config"
	"github.com/tiramission/oci-sync/internal/oci"
)

// The GUI is a view function over app state. MyGo calls it again after every
// input and after Window.Update, and a single event may run several passes, so
// state must hold only data, ui.Handle and ListState — never an Element — and
// every remote call runs in a goroutine that hands its result back through
// Window.Update. Triggering work from rendering code would fire it several
// times per click.

// guiOpKind is the operation a modal is driving, if any.
type guiOpKind int

const (
	guiOpNone guiOpKind = iota
	guiOpPull
	guiOpDelete
)

// guiModal is the state of the overlay on top of the artifact table.
type guiModal struct {
	kind    guiOpKind
	tag     string
	repo    string
	title   string
	confirm string

	destPath   string
	passphrase string
	// passphraseShown is false for a plain artifact, so the prompt asks
	// for a key only when the artifact actually needs one.
	passphraseShown bool

	busy bool
	// err holds a failure from the last attempt. The modal stays open so
	// the user can correct the passphrase and retry.
	err error
}

// guiProgress is a pull's progress, written from the download goroutine and
// read while rendering. It carries its own lock rather than living in
// guiApp, which is only safe to touch on the main thread.
type guiProgress struct {
	mu    sync.Mutex
	phase string
	done  int64
	total int64
}

func (p *guiProgress) set(stage PullStage) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.phase = stage.Phase
	p.done = stage.Done
	p.total = stage.Total
}

func (p *guiProgress) snapshot() (string, int64, int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.phase, p.done, p.total
}

// guiApp is the whole application state. It is created once, before the view
// exists, and every field but progress is mutated on the main thread only.
type guiApp struct {
	shortcuts []config.ShortcutInfo

	// navList holds the shortcut selection and tableList the artifact
	// selection. Both need a Selected pointer that outlives the frame, and
	// MyGo identifies rows through Key so a choice survives a reload.
	navList   ui.ListState
	navSel    int
	tableList ui.ListState
	tableSel  int

	repo      string
	artifacts []oci.ArtifactInfo

	loading bool
	listErr error

	// modalOpen is the *bool ui.Modal watches: it has to be app state, a
	// local would be a fresh false on every pass and the dialog would
	// never appear.
	modalOpen bool
	modal     guiModal
	progress  guiProgress

	// filter is bound directly to the TextInput, so it is already current
	// in the frame that Changed reports.
	filter string

	// jobSeq invalidates stale async results: a fetch that finishes after
	// the user switched shortcuts must not overwrite the new selection.
	jobSeq int

	win *mygo.Window
}

func newGuiApp(shortcuts []config.ShortcutInfo) *guiApp {
	a := &guiApp{shortcuts: shortcuts}
	// Handle is usable as a zero value; naming each list keeps MyGo from
	// confusing the two when it tracks focus across passes.
	a.navList = ui.ListState{
		Handle:   ui.Handle{},
		Selected: &a.navSel,
	}
	a.tableList = ui.ListState{
		Handle:   ui.Handle{},
		Selected: &a.tableSel,
	}
	return a
}

// guiVisible returns the artifacts matching the filter, so the table shows
// only what the filter keeps.
func (a *guiApp) guiVisible() []oci.ArtifactInfo {
	if a.filter == "" {
		return a.artifacts
	}
	needle := strings.ToLower(a.filter)
	out := make([]oci.ArtifactInfo, 0, len(a.artifacts))
	for _, x := range a.artifacts {
		if strings.Contains(strings.ToLower(x.Tag), needle) {
			out = append(out, x)
		}
	}
	return out
}

func (a *guiApp) currentShortcut() config.ShortcutInfo {
	if a.navSel < 0 || a.navSel >= len(a.shortcuts) {
		return config.ShortcutInfo{}
	}
	return a.shortcuts[a.navSel]
}

// selectedArtifact is the artifact the action buttons apply to, or false when
// the table is empty or the choice points past the filtered rows.
func (a *guiApp) selectedArtifact() (oci.ArtifactInfo, bool) {
	rows := a.guiVisible()
	if a.tableSel < 0 || a.tableSel >= len(rows) {
		return oci.ArtifactInfo{}, false
	}
	return rows[a.tableSel], true
}

func runGui(ctx context.Context) error {
	// The TUI and the GUI must not fight over the terminal: MyGo owns the
	// console once its window opens.
	log.SetLevel(log.ErrorLevel)

	shortcuts := config.GetAllShortcuts()
	if len(shortcuts) == 0 {
		fmt.Println("No shortcuts configured. Use 'oci-sync alias add <name> --repo <repo>' to add one first.")
		return nil
	}

	app := newGuiApp(shortcuts)

	mygo.App.WhenReady(func() {
		app.win = mygo.NewWindow(mygo.WindowOptions{
			Title:     "oci-sync",
			Width:     1000,
			Height:    640,
			MinWidth:  720,
			MinHeight: 420,
			Content:   ui.View(app.view),
		})
		app.refresh()
	})

	// A cancelled context means the caller wants us gone; Quit closes the
	// window, which is what App.Run waits for.
	if ctx != nil && ctx.Done() != nil {
		go func() {
			<-ctx.Done()
			mygo.App.Quit()
		}()
	}

	if err := mygo.App.Run(); err != nil {
		return fmt.Errorf("gui failed: %w", err)
	}
	return nil
}

// refresh reloads the current shortcut's artifacts in the background.
func (a *guiApp) refresh() {
	repo := a.currentShortcut().Repo
	if repo == "" {
		return
	}

	a.jobSeq++
	seq := a.jobSeq
	a.loading = true
	a.listErr = nil

	win := a.win
	go func() {
		artifacts, err := oci.List(context.Background(), repo)
		if win == nil {
			return
		}
		win.Update(func() {
			// A newer request already started, or the window is gone.
			if seq != a.jobSeq {
				return
			}
			a.loading = false
			a.repo = repo
			if err != nil {
				a.listErr = err
				a.artifacts = nil
				return
			}
			a.listErr = nil
			a.artifacts = artifacts
			// The choice is an index into the filtered rows, so it has to
			// be clamped after every reload, not only on shortcut switch.
			if max := len(a.guiVisible()) - 1; a.tableSel > max {
				a.tableSel = max
			}
			if a.tableSel < 0 {
				a.tableSel = 0
			}
		})
	}()
}

// invalidate asks for a frame without waiting, so a progress tick reaches
// the screen without rebuilding from an action.
func (a *guiApp) invalidate() {
	if a.win != nil {
		a.win.Invalidate()
	}
}

// startPull runs a pull in the background and reports through the modal.
// The ref goes through buildShortcutRemoteRef so the GUI and the
// per-shortcut subcommands agree on how a name becomes a reference.
func (a *guiApp) startPull() {
	name := a.modal.repo
	tag := a.modal.tag
	dest := a.modal.destPath
	pass := a.modal.passphrase

	a.progress.set(PullStage{Phase: "check"})
	win := a.win
	go func() {
		ref, err := buildShortcutRemoteRef(name, tag)
		if err != nil {
			a.failModal(err)
			return
		}
		err = runPull(context.Background(), ref, dest, pass, func(stage PullStage) {
			a.progress.set(stage)
			a.invalidate()
		})
		if win == nil {
			return
		}
		win.Update(func() {
			if err != nil {
				a.finishModal(err)
				return
			}
			a.closeModal()
			a.record(cache.ActivityPull, ref, dest, nil)
			a.refresh()
		})
	}()
}

// startDelete removes a remote tag in the background.
func (a *guiApp) startDelete() {
	name := a.modal.repo
	tag := a.modal.tag

	win := a.win
	go func() {
		ref, err := buildShortcutRemoteRef(name, tag)
		if err != nil {
			a.failModal(err)
			return
		}
		err = runDelete(context.Background(), ref)
		if win == nil {
			return
		}
		win.Update(func() {
			if err != nil {
				a.finishModal(err)
				return
			}
			a.closeModal()
			a.record(cache.ActivityDelete, ref, "", nil)
			a.refresh()
		})
	}()
}

// failModal reports a failure that happened before win was usable, so the
// modal cannot go through Update to do it.
func (a *guiApp) failModal(err error) {
	if a.win == nil {
		return
	}
	a.win.Update(func() { a.finishModal(err) })
}

func (a *guiApp) finishModal(err error) {
	a.modal.busy = false
	a.modal.err = err
}

// record writes an activity entry. A broken cache must not turn a successful
// pull into a reported failure, so a write error only warns.
func (a *guiApp) record(kind cache.ActivityType, ref, dest string, labels []string) {
	recordActivity(cache.Activity{
		Type:      kind,
		Timestamp: time.Now(),
		RemoteRef: ref,
		LocalPath: dest,
		Labels:    labels,
		Success:   true,
	})
}

func (a *guiApp) closeModal() {
	a.modalOpen = false
	a.modal = guiModal{}
}

// view builds one frame. Every branch below must stay free of I/O.
func (a *guiApp) view(c *ui.Context) {
	ui.Column(c).Fill().Gap(0).Children(func() {
		a.viewToolbar(c)
		ui.Row(c).Grow(1).FillWidth().Gap(0).Children(func() {
			a.viewSidebar(c)
			a.viewMain(c)
		})
		a.viewModal(c)
	})
}

func (a *guiApp) viewToolbar(c *ui.Context) {
	th := c.Theme()
	// FillWidth, not Fill: Fill claims the parent's whole height too and
	// centres the row in it, which would push everything below off screen.
	ui.Row(c).FillWidth().Gap(8).Padding(8, 12).Background(th.Surface).Children(func() {
		ui.Text(c, "oci-sync").Bold().FontSize(15)

		ui.Row(c).Grow(1).Gap(8).Justify(ui.End).Children(func() {
			ui.Text(c, "Filter").TextColor(th.TextMuted)
			ui.TextInput(c, &a.filter).
				Placeholder("tag contains…").
				SingleLine().
				Width(220)
			if a.loading {
				ui.Text(c, "Loading…").TextColor(th.TextMuted)
			}
			if ui.Button(c, "Refresh").Clicked() {
				a.refresh()
			}
		})
	})
}

func (a *guiApp) viewSidebar(c *ui.Context) {
	th := c.Theme()
	// The list has to Grow and the column FillHeight, or the list takes its
	// natural height and pushes the rest of the window out of view.
	ui.Column(c).FillHeight().Width(220).Gap(6).Padding(10).Children(func() {
		ui.Text(c, "SHORTCUTS").TextColor(th.TextMuted).FontSize(11)

		list := ui.List(c, &a.navList, len(a.shortcuts))
		list.ItemKey(func(row int) any {
			if row < len(a.shortcuts) {
				return a.shortcuts[row].Name
			}
			return nil
		})
		// Rows is what lets a row know it is the chosen one; without it the
		// list falls back to the plain row callback.
		list.Rows(func(r ui.ListRow) {
			if r.Index >= len(a.shortcuts) {
				return
			}
			s := a.shortcuts[r.Index]
			bg := th.Background
			if r.Selected() {
				bg = th.Selection
			}
			ui.Column(c).FillWidth().Gap(0).Padding(4, 6).Background(bg).Children(func() {
				// Bold takes no argument, so a conditional style has to
				// branch on the whole call: an Element is registered with
				// the build pass by the call that creates it.
				if r.Selected() {
					ui.Text(c, s.Name).Bold().MaxLines(1).Ellipsis("…")
				} else {
					ui.Text(c, s.Name).MaxLines(1).Ellipsis("…")
				}
				ui.Text(c, s.Repo).
					TextColor(th.TextMuted).
					FontSize(11).
					SingleLine().
					Ellipsis("…")
			})
		})
		// Grow last, so the list takes the room the header leaves. Without
		// it the rows would sit at their natural height.
		list.Grow(1).FillHeight()

		if list.Changed() {
			a.selectShortcut(a.navSel)
		}
	})
}

func (a *guiApp) selectShortcut(row int) {
	if row < 0 || row >= len(a.shortcuts) {
		return
	}
	// The List writes navSel through its Selected pointer before Changed
	// reports, but setting it here too keeps this function correct when it
	// is called from anywhere else.
	a.navSel = row
	a.artifacts = nil
	a.listErr = nil
	a.filter = ""
	// A selection from the previous repo would point at an unrelated row.
	a.tableSel = 0
	a.refresh()
}

func (a *guiApp) viewMain(c *ui.Context) {
	th := c.Theme()
	ui.Column(c).Grow(1).FillWidth().FillHeight().Gap(8).Padding(12).Children(func() {
		head := a.repo
		if head == "" {
			head = a.currentShortcut().Repo
		}
		rows := a.guiVisible()
		ui.Text(c, fmt.Sprintf("%s — %d tag(s)", head, len(rows))).TextColor(th.TextMuted)

		// Exactly one of these takes the room the header and the action row
		// leave. Without Grow/FillHeight on the table it would claim its
		// natural height and push the buttons off the bottom of the window,
		// where they cannot be clicked.
		switch {
		case a.listErr != nil:
			ui.Text(c, "Cannot list tags: "+a.listErr.Error()).TextColor(th.Danger)
			ui.Spacer(c)
		case a.loading && len(rows) == 0:
			ui.Text(c, "Loading…").TextColor(th.TextMuted)
			ui.Spacer(c)
		case len(rows) == 0:
			ui.Text(c, "No artifacts. Push one with 'oci-sync push', or clear the filter.").
				TextColor(th.TextMuted)
			ui.Spacer(c)
		default:
			cols := []ui.TableColumn{
				{Title: "TAG", ID: "tag", Width: 240},
				{Title: "SIZE", ID: "size", Width: 100, Align: ui.End},
				{Title: "ENC", ID: "enc", Width: 60},
				{Title: "VERSION", ID: "version", Width: 90},
			}
			ui.Table(c, &a.tableList, cols, len(rows), func(row, col int) {
				x := rows[row]
				switch col {
				case 0:
					ui.Text(c, x.Tag).MaxLines(1).Ellipsis("…")
				case 1:
					ui.Text(c, formatBytes(int(x.Size)))
				case 2:
					if x.Encrypted {
						ui.Text(c, "yes").TextColor(th.Warning)
					} else {
						ui.Text(c, "no").TextColor(th.TextMuted)
					}
				case 3:
					ui.Text(c, x.Version)
				}
			}).Grow(1).FillHeight()
		}

		a.viewActions(c)
	})
}

func (a *guiApp) viewActions(c *ui.Context) {
	th := c.Theme()
	ui.Row(c).Gap(8).Children(func() {
		x, ok := a.selectedArtifact()
		if !ok {
			ui.Text(c, "Select a tag to pull or delete.").TextColor(th.TextMuted)
			return
		}
		if ui.PrimaryButton(c, "Pull…").Clicked() {
			a.openPullModal(x)
		}
		// ButtonBase has no danger look of its own, so the destructive
		// action is a plain button painted with the theme's Danger.
		if ui.ButtonBase(c).
			Padding(6, 14).
			Radius(th.Radius).
			Background(th.Surface).
			Border(1, th.Border).
			Children(func() {
				ui.Text(c, "Delete…").TextColor(th.Danger)
			}).Clicked() {
			a.openDeleteModal(x)
		}
	})
}

func (a *guiApp) openPullModal(x oci.ArtifactInfo) {
	shortcut := a.currentShortcut().Name
	a.modal = guiModal{
		kind:    guiOpPull,
		tag:     x.Tag,
		repo:    shortcut,
		title:   "Pull " + x.Tag,
		confirm: "Pull",
		// Land beside the working directory rather than in the registry's
		// namespace, which is rarely what the user wants to untar into.
		destPath: "./" + x.Tag,
		// Only ask for a key when the artifact is actually encrypted.
		passphraseShown: x.Encrypted,
	}
	a.progress.set(PullStage{})
	a.modalOpen = true
}

func (a *guiApp) openDeleteModal(x oci.ArtifactInfo) {
	a.modal = guiModal{
		kind:    guiOpDelete,
		tag:     x.Tag,
		repo:    a.currentShortcut().Name,
		title:   "Delete " + x.Tag,
		confirm: "Delete",
	}
	a.modalOpen = true
}

// browse asks the OS for a directory. The native dialog blocks, so it runs
// off the main thread and the answer comes back through Window.Update.
func (a *guiApp) browse(dest *string) {
	win := a.win
	go func() {
		dirs, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:      win,
			Title:       "Choose a destination directory",
			Directory:   true,
			DefaultPath: *dest,
		})
		if win == nil {
			return
		}
		win.Update(func() {
			if err == nil && len(dirs) > 0 {
				*dest = dirs[0]
			}
		})
	}()
}

func (a *guiApp) viewModal(c *ui.Context) {
	if !a.modalOpen {
		return
	}
	th := c.Theme()
	m := &a.modal

	modal := ui.Modal(c, &a.modalOpen, func() {
		ui.Column(c).FillWidth().Gap(10).Padding(16).
			Background(th.Background).
			Border(1, th.Border).
			Radius(th.Radius).
			Children(func() {
				ui.Text(c, m.title).Bold().FontSize(15)

				switch m.kind {
				case guiOpPull:
					ui.Text(c, "Destination").TextColor(th.TextMuted)
					ui.Row(c).Gap(8).Children(func() {
						ui.TextInput(c, &m.destPath).Grow(1).SingleLine()
						if ui.Button(c, "Browse…").Clicked() {
							a.browse(&m.destPath)
						}
					})
					if m.passphraseShown {
						ui.Text(c, "Passphrase").TextColor(th.TextMuted)
						ui.TextInput(c, &m.passphrase).Password().SingleLine()
					}
				case guiOpDelete:
					ui.Text(c, "This removes the tag from the remote registry and cannot be undone.")
				}

				if m.busy && m.kind == guiOpPull {
					a.viewProgress(c)
				}
				if m.err != nil {
					ui.Text(c, m.err.Error()).TextColor(th.Danger)
				}

				ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
					if m.busy {
						ui.Text(c, "Working…").TextColor(th.TextMuted)
						return
					}
					if ui.Button(c, "Cancel").Clicked() {
						a.closeModal()
					}
					label := m.confirm
					if ui.PrimaryButton(c, label).Clicked() {
						m.busy = true
						m.err = nil
						if m.kind == guiOpPull {
							a.startPull()
						} else {
							a.startDelete()
						}
					}
				})
			})
	})
	// Dismissed covers Escape and a click outside the dialog.
	if modal.Dismissed() {
		a.closeModal()
	}
}

func (a *guiApp) viewProgress(c *ui.Context) {
	th := c.Theme()
	phase, done, total := a.progress.snapshot()
	label := phase
	switch phase {
	case "download":
		if total > 0 {
			label = fmt.Sprintf("downloading %s / %s", formatBytes(int(done)), formatBytes(int(total)))
		} else {
			label = "downloading"
		}
	case "decrypt":
		label = "decrypting"
	case "unpack":
		label = "unpacking"
	case "check":
		label = "checking"
	}
	if label == "" {
		return
	}
	ui.Text(c, label).TextColor(th.TextMuted)
	if phase == "download" && total > 0 {
		ui.Progress(c, float64(done)/float64(total)).FillWidth()
	}
}
