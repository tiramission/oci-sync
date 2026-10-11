package cmd

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

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
//
// Every string the user reads is written in Chinese, including validation
// errors shown inside a dialog. The CLI stays English (see AGENTS.md), so
// these literals live here rather than in a shared table: an error crossing
// between the two would have to pick one language or carry both.

// guiOpKind is the operation a modal is driving, if any.
type guiOpKind int

const (
	guiOpNone guiOpKind = iota
	guiOpPull
	guiOpDelete
	guiOpPush
	guiOpLabel
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

	// srcPath and labels only apply to a push; the fields are shared with
	// the pull dialog rather than split into a second struct, because the
	// two dialogs are the same shape.
	srcPath string
	// labels is one "key=value" per line, which is also how it is passed
	// to runPush, so what the user typed is what gets parsed.
	labels string

	// labelEdits is the same shape as labels but for the label editor, and
	// currentLabels is the artifact's labels as found, so the dialog can
	// show what is there now. They are separate from labels because a push
	// and a label edit never run together.
	labelEdits    string
	currentLabels []string

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
		fmt.Println("尚未配置任何快捷方式。请先执行 'oci-sync alias add <名称> --repo <仓库>' 添加一个。")
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
			a.refresh()
		})
	}()
}

// startPush runs a push in the background and reports through the modal.
// Packing and uploading are both buffered whole, so there is no byte-level
// progress to show — only that the operation is under way.
func (a *guiApp) startPush() {
	name := a.modal.repo
	tag := a.modal.tag
	src := a.modal.srcPath
	pass := a.modal.passphrase
	labels := a.modal.pushLabels()

	a.progress.set(PullStage{Phase: "pack"})
	win := a.win
	go func() {
		ref, err := buildShortcutRemoteRef(name, tag)
		if err != nil {
			a.failModal(err)
			return
		}
		err = runPush(context.Background(), src, ref, pass, labels)
		if win == nil {
			return
		}
		win.Update(func() {
			if err != nil {
				a.finishModal(err)
				return
			}
			a.closeModal()
			// The new tag has to appear, and the repo may be one the list
			// has not loaded yet, so refresh rather than patch the slice.
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

// startLabel applies the editor buffer: entries with '=' set or update a
// label, bare keys remove one, which covers both halves of the CLI's label
// set / unset pair in a single field.
func (a *guiApp) startLabel() {
	shortcut := a.currentShortcut().Name
	tag := a.modal.tag
	edits := a.modal.labelEdits
	win := a.win
	go func() {
		ref, err := buildShortcutRemoteRef(shortcut, tag)
		if err != nil {
			a.failModal(err)
			return
		}
		var sets, unsets []string
		for _, entry := range splitLabels(edits) {
			if strings.Contains(entry, "=") {
				sets = append(sets, entry)
			} else {
				unsets = append(unsets, entry)
			}
		}
		// Set before unset: removing a key this same edit just added would
		// be surprising, and the order also matches how the two CLI commands
		// read.
		if len(sets) > 0 {
			if err := runLabelSet(context.Background(), ref, sets); err != nil {
				a.failModal(err)
				return
			}
		}
		if len(unsets) > 0 {
			if err := runLabelUnset(context.Background(), ref, unsets); err != nil {
				a.failModal(err)
				return
			}
		}
		if win == nil {
			return
		}
		win.Update(func() {
			a.closeModal()
			// The digest moved when annotations changed, so the row has to
			// be re-listed rather than patched in place.
			a.refresh()
		})
	}()
}

func (a *guiApp) finishModal(err error) {
	a.modal.busy = false
	a.modal.err = err
}

func (a *guiApp) closeModal() {
	a.modalOpen = false
	a.modal = guiModal{}
}

// view builds one frame. Every branch below must stay free of I/O.
func (a *guiApp) view(c *ui.Context) {
	// The theme is retuned here rather than once at startup so it follows
	// the window between light and dark while it stays open.
	applyDesignTheme(c.Theme())
	ui.Column(c).Fill().Gap(0).Children(func() {
		a.viewToolbar(c)
		ui.Row(c).Grow(1).FillWidth().Gap(0).Children(func() {
			a.viewSidebar(c)
			a.viewMain(c)
		})
		a.viewStatusBar(c)
		a.viewModal(c)
	})
}

// viewStatusBar is a permanent strip along the bottom. The activity cache
// already records every operation, but reaching it means another command;
// the last action is the thing users check most often.
func (a *guiApp) viewStatusBar(c *ui.Context) {
	th := c.Theme()
	// FillWidth, not Fill: Fill would claim the parent column's height too.
	ui.Row(c).FillWidth().Gap(8).Padding(7, 14).Background(th.Surface).Children(func() {
		ui.Text(c, a.statusLine(th)).
			TextColor(readableSecondary(th)).FontSize(12).MaxLines(1).Ellipsis("…")
	})
}

// statusLine describes where the app is: what just happened, or what the
// user is looking at when nothing has happened yet.
func (a *guiApp) statusLine(th *ui.Theme) string {
	switch {
	case a.listErr != nil:
		return "加载失败：" + a.listErr.Error()
	case a.loading:
		return "正在加载 " + a.currentShortcut().Repo + "…"
	}
	if line := lastActivityLine(); line != "" {
		return line
	}
	if x, ok := a.selectedArtifact(); ok {
		return fmt.Sprintf("已选中 %s · %s · %s", x.Tag, formatBytes(x.Size),
			map[bool]string{true: "已加密", false: "未加密"}[x.Encrypted])
	}
	return "就绪。选择一个仓库开始。"
}

// lastActivityLine renders the newest cache entry for the status bar. The
// wording lives here rather than in internal/cache: that package is the
// store and knows nothing about any interface language.
func lastActivityLine() string {
	acts, err := cache.GetRecentActivities(1)
	if err != nil || len(acts) == 0 {
		return ""
	}
	act := acts[0]
	when := act.Timestamp.Format("15:04")
	if act.Success {
		return fmt.Sprintf("%s %s %s", when, activityVerb(act.Type), act.RemoteRef)
	}
	return fmt.Sprintf("%s %s %s 失败", when, activityVerb(act.Type), act.RemoteRef)
}

func activityVerb(t cache.ActivityType) string {
	switch t {
	case cache.ActivityPush:
		return "已推送"
	case cache.ActivityPull:
		return "已拉取"
	case cache.ActivityDelete:
		return "已删除"
	case cache.ActivityLabel:
		return "已改标签"
	default:
		return "操作"
	}
}

func (a *guiApp) viewToolbar(c *ui.Context) {
	th := c.Theme()
	// FillWidth, not Fill: Fill claims the parent's whole height too and
	// centres the row in it, which would push everything below off screen.
	ui.Row(c).FillWidth().Gap(8).Padding(8, 12).Background(th.Surface).Children(func() {
		ui.Text(c, "oci-sync").Bold().FontSize(15)

		ui.Row(c).Grow(1).Gap(8).Justify(ui.End).Children(func() {
			// The toolbar sits on Surface, not Background, and TextMuted on
			// Surface is the one pair the light palette puts below the WCAG
			// AA threshold for body text (4.40:1). Secondary labels here
			// have to be readable, so they get readableSecondary instead.
			ui.Text(c, "过滤").TextColor(readableSecondary(th))
			ui.TextInput(c, &a.filter).
				Placeholder("tag 包含…").
				SingleLine().
				Width(220)
			if a.loading {
				ui.Text(c, "加载中…").TextColor(readableSecondary(th))
			}
			if ui.Button(c, "刷新").Clicked() {
				a.refresh()
			}
		})
	})
}

func (a *guiApp) viewSidebar(c *ui.Context) {
	th := c.Theme()
	// The list has to Grow and the column FillHeight, or the list takes its
	// natural height and pushes the rest of the window out of view.
	ui.Column(c).FillHeight().Width(232).Gap(6).Padding(12, 10).Children(func() {
		ui.Text(c, "快捷方式").
			TextColor(readableSecondary(th)).
			FontSize(12).
			Bold()

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
			// The selection is a tint rather than a full-strength block: a
			// solid fill over a multi-row table shouts louder than the state
			// deserves, and it forces every child to find a colour that
			// survives it.
			bg := th.Background
			nameColor := th.Text
			if r.Selected() {
				bg = th.Selection
				nameColor = violet600
			}
			ui.Column(c).FillWidth().Gap(3).Padding(8, 10).Background(bg).Children(func() {
				// Name and count share a row so the count reads as a badge
				// on the repository rather than as another field.
				ui.Row(c).FillWidth().Gap(6).Children(func() {
					// Bold takes no argument, so a conditional style has to
					// branch on the whole call: an Element is registered with
					// the build pass by the call that creates it.
					if r.Selected() {
						ui.Text(c, s.Name).Bold().TextColor(nameColor).
							MaxLines(1).Ellipsis("…").Grow(1)
					} else {
						ui.Text(c, s.Name).TextColor(nameColor).
							MaxLines(1).Ellipsis("…").Grow(1)
					}
					if n := a.shortcutCount(r.Index); n > 0 {
						ui.Text(c, fmt.Sprintf("%d", n)).
							TextColor(readableSecondary(th)).
							FontSize(11)
					}
				})
				// The repo line is the one that identifies where this
				// shortcut points, so it stays legible on both row
				// backgrounds — muted grey over the selection tint is the
				// worst pair in the window at 3.37:1.
				repoColor := readableSecondary(th)
				if r.Selected() {
					repoColor = readableOnSelection(th)
				}
				ui.Text(c, s.Repo).
					TextColor(repoColor).
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

// shortcutCount is the badge beside a repository name. Only the repository
// that has actually been listed shows a number; the rest would be zeroes
// that mean "never loaded" rather than "empty", which is a different thing.
func (a *guiApp) shortcutCount(row int) int {
	if row != a.navSel {
		return 0
	}
	return len(a.artifacts)
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
		// A repository with nothing in it and a filter that hides
		// everything both render as an empty table, but they need
		// opposite actions: push, or clear the filter.
		headSuffix := fmt.Sprintf("共 %d 个 tag", len(rows))
		if len(rows) != len(a.artifacts) {
			headSuffix = fmt.Sprintf("匹配 %d / %d 个 tag", len(rows), len(a.artifacts))
		}
		ui.Text(c, fmt.Sprintf("%s — %s", head, headSuffix)).TextColor(readableSecondary(th))

		// Exactly one of these takes the room the header and the action row
		// leave. Without Grow/FillHeight on the table it would claim its
		// natural height and push the buttons off the bottom of the window,
		// where they cannot be clicked.
		switch {
		case a.listErr != nil:
			ui.Text(c, "无法列出 tag："+a.listErr.Error()).TextColor(th.Danger)
			// A failed listing is the state most likely to be transient
			// (expired credentials, a network blip), so the recovery is
			// one click rather than a restart.
			if ui.Button(c, "重试").Clicked() {
				a.refresh()
			}
			ui.Spacer(c)
		case a.loading && len(rows) == 0:
			ui.Text(c, "加载中…").TextColor(readableSecondary(th))
			ui.Spacer(c)
		case len(rows) == 0 && len(a.artifacts) > 0:
			ui.Text(c, "没有 tag 匹配「"+a.filter+"」。").TextColor(readableSecondary(th))
			if ui.Button(c, "清除过滤").Clicked() {
				a.filter = ""
				a.invalidate()
			}
			ui.Spacer(c)
		case len(rows) == 0:
			ui.Text(c, "此仓库还没有 artifact。").TextColor(readableSecondary(th))
			ui.Spacer(c)
		default:
			a.viewArtifactTable(c, rows)
		}

		a.viewActions(c)
	})
}

// viewArtifactTable draws the artifact grid.
//
// It is a ui.List drawn by hand rather than a ui.Table, because the design
// system asks for a tinted selected row with an accent bar down its left
// edge, and ui.Table paints the chosen row in a solid Accent block with no
// hook to restyle it. Over a three-row grid that block is a slab of colour
// for a state the user is only glancing at, and it drags every cell colour
// along with it. Drawing the rows keeps the tint, and costs the column
// resize handles — a grid this narrow does not need them.
//
// Column widths are fixed rather than measured: the tag column takes the
// room the others leave, which is what lets a long tag ellipsize instead of
// pushing the numbers off the edge.
func (a *guiApp) viewArtifactTable(c *ui.Context, rows []oci.ArtifactInfo) {
	th := c.Theme()
	ui.Column(c).Grow(1).FillHeight().Gap(0).Children(func() {
		a.artifactHeader(c)

		list := ui.List(c, &a.tableList, len(rows))
		// Keying on the tag rather than the index keeps the chosen row
		// pointed at the same artifact after a reload reorders the slice.
		list.ItemKey(func(row int) any {
			if row < len(rows) {
				return rows[row].Tag
			}
			return nil
		})
		list.Rows(func(r ui.ListRow) {
			if r.Index >= len(rows) {
				return
			}
			x := rows[r.Index]
			bg := th.Background
			// Text on the chosen row has to be re-picked, not inherited:
			// the muted grey that reads well on white is the worst pair
			// against the selection tint, and the warning amber for
			// "encrypted" washes out completely.
			ink, inkMuted := th.Text, readableSecondary(th)
			if r.Selected() {
				bg = th.Selection
				ink = violet700
				inkMuted = readableOnSelection(th)
			}
			ui.Row(c).FillWidth().AlignItems(ui.Center).
				Padding(6, 10).Gap(10).Background(bg).Children(func() {
				// The accent bar: a narrow rule at the row's own left
				// edge, which is how the design marks the chosen row
				// without recolouring the whole strip. It is a Box and
				// not a Spacer: Spacer grows to fill, and Width does not
				// take that back, so a Spacer bar would lie across the
				// whole row instead of standing at its edge.
				bar := bg
				if r.Selected() {
					bar = violet600
				}
				ui.Box(c).Width(2).Height(14).Background(bar)

				ui.Text(c, x.Tag).Grow(1).MaxLines(1).Ellipsis("…").TextColor(ink)
				ui.Text(c, formatBytes(x.Size)).Width(90).
					Justify(ui.End).MaxLines(1).TextColor(inkMuted)
				// "是" marks an encrypted artifact, the one cell worth
				// noticing in a scan, so it takes the readable warning
				// colour; "否" is the absence of news and stays quiet.
				// The warning is resolved against the row's own
				// background: the amber that clears AA on white does not
				// clear it on the selection tint.
				if x.Encrypted {
					ui.Text(c, "是").Width(50).
						TextColor(readableWarningOn(th, bg)).Bold()
				} else {
					ui.Text(c, "否").Width(50).TextColor(inkMuted)
				}
				ui.Text(c, x.Version).Width(80).MaxLines(1).TextColor(ink)
			})
		})
		list.Grow(1).FillHeight()
	})
}

// artifactHeader heads the grid. It sits on the tinted surface the design
// gives the header band, and right-aligns the size column to match the cells
// below it — a header that does not line up with its column is the fastest
// way to make a grid hard to read.
func (a *guiApp) artifactHeader(c *ui.Context) {
	th := c.Theme()
	label := func(s string) ui.Element {
		return ui.Text(c, s).TextColor(readableSecondary(th)).FontSize(11.5).Bold()
	}
	ui.Row(c).FillWidth().AlignItems(ui.Center).
		Padding(6, 12).Gap(10).Background(th.Surface).Children(func() {
		// The 2px rule matches the row's accent bar, so the header sits
		// over the cells rather than half a cell to the right of them.
		ui.Box(c).Width(2).Height(1)
		label("标签").Grow(1).Justify(ui.Start)
		label("大小").Width(90).Justify(ui.End)
		label("加密").Width(50)
		label("版本").Width(80)
	})
}

// viewActions lays the four operations out under the grid.
//
// The order is the design system's: push, pull, delete, labels. Delete sits
// between the two reversible row actions rather than last because it is the
// only one here that cannot be undone — putting it on the far side of the
// row, next to nothing, is what keeps a slip from costing data.
//
// A hairline separates the row from the grid, so the buttons read as a
// toolbar rather than as more table content. The column does not Grow: the
// grid above already takes the slack, and a second Grow here would take
// half of it back, parking the buttons in the middle of the window with a
// field of empty space below them. Staying at its natural height is what
// puts the row against the bottom edge.
func (a *guiApp) viewActions(c *ui.Context) {
	th := c.Theme()
	ui.Column(c).FillWidth().Gap(14).Children(func() {
		ui.Box(c).FillWidth().Height(1).Background(th.Border)
		a.viewActionButtons(c)
	})
}

// viewActionButtons is the button row itself, without the rule above it.
func (a *guiApp) viewActionButtons(c *ui.Context) {
	th := c.Theme()
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
		// Push targets the repository rather than a row, so it stays
		// available when nothing is selected.
		if ui.Button(c, "推送…").Clicked() {
			a.openPushModal()
		}

		x, ok := a.selectedArtifact()
		if !ok {
			ui.Text(c, "请选择一个 tag 以拉取、改标签或删除。").
				TextColor(readableSecondary(th)).Grow(1)
			return
		}
		if ui.PrimaryButton(c, "拉取…").Clicked() {
			a.openPullModal(x)
		}
		// ButtonBase has no danger look of its own, so the destructive
		// action is a plain button painted with the theme's Danger.
		if a.dangerButton(c, "删除…").Clicked() {
			a.openDeleteModal(x)
		}
		if ui.Button(c, "编辑标签…").Clicked() {
			a.openLabelModal(x)
		}
		// MaxLines(1) because this row shares its height with the buttons: a
		// summary that wraps turns the toolbar two rows tall and shoves
		// the buttons off the bottom of a short window. A narrow window
		// drops the clause rather than the line — the tag is what identifies
		// the row, the size and the encryption flag are detail.
		ui.Text(c, selectedSummary(x)).
			TextColor(readableSecondary(th)).
			MaxLines(1).Ellipsis("…").
			Grow(1).Justify(ui.End)
	})
}

// selectedSummary describes the row the row-scoped actions will act on, so
// the user never has to look back up the grid to check which tag "it" is.
func selectedSummary(x oci.ArtifactInfo) string {
	s := x.Tag + " · " + formatBytes(x.Size)
	if x.Encrypted {
		return s + " · 已加密"
	}
	return s
}

// dangerButton is a plain button painted with the theme's Danger color, for
// actions that cannot be undone. MyGo has no DangerButton, and ButtonBase is
// the only way to draw a button that is neither primary nor plain.
func (a *guiApp) dangerButton(c *ui.Context, label string) ui.Element {
	th := c.Theme()
	return ui.ButtonBase(c).
		Padding(6, 14).
		Radius(th.Radius).
		Background(th.Surface).
		Border(1, th.Border).
		Children(func() {
			ui.Text(c, label).TextColor(th.Danger)
		})
}

func (a *guiApp) openPullModal(x oci.ArtifactInfo) {
	shortcut := a.currentShortcut().Name
	a.modal = guiModal{
		kind:    guiOpPull,
		tag:     x.Tag,
		repo:    shortcut,
		title:   "拉取 " + x.Tag,
		confirm: "拉取",
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
		title:   "删除 " + x.Tag,
		confirm: "删除",
	}
	a.modalOpen = true
}

// openLabelModal opens the label editor for an artifact. It lists the labels
// as found so the user edits against reality rather than from memory.
func (a *guiApp) openLabelModal(x oci.ArtifactInfo) {
	keys := make([]string, 0, len(x.Labels))
	for k := range x.Labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	current := make([]string, 0, len(keys))
	for _, k := range keys {
		current = append(current, k+"="+x.Labels[k])
	}
	a.modal = guiModal{
		kind:          guiOpLabel,
		tag:           x.Tag,
		repo:          a.currentShortcut().Name,
		title:         "标签 · " + x.Tag,
		confirm:       "应用",
		currentLabels: current,
	}
	a.modalOpen = true
}

// openPushModal opens the push dialog. It targets the current shortcut's
// repository, so it stays available even before any artifact is listed.
func (a *guiApp) openPushModal() {
	a.modal = guiModal{
		kind:    guiOpPush,
		repo:    a.currentShortcut().Name,
		title:   "推送到 " + a.currentShortcut().Repo,
		confirm: "推送",
		// The tag has to be typed: there is nothing on the remote side to
		// suggest one, and overwriting a tag by accident is worse than
		// typing it out.
		tag: "latest",
	}
	a.progress.set(PullStage{})
	a.modalOpen = true
}

// pushLabels splits the label lines into the "key=value" form runPush takes.
// A blank line is skipped so a trailing newline is harmless.
func (m *guiModal) pushLabels() []string {
	var out []string
	for _, line := range strings.Split(m.labels, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return out
}

// validatePush reports what the dialog still needs before a push can run.
// Checking here keeps the failure next to the field that caused it, rather
// than surfacing later as an opaque error from the registry round trip.
func (m *guiModal) validatePush() error {
	if strings.TrimSpace(m.srcPath) == "" {
		return errors.New("请选择要推送的文件或目录")
	}
	if strings.TrimSpace(m.tag) == "" {
		return errors.New("tag 不能为空")
	}
	if strings.ContainsAny(m.tag, " \t") {
		return fmt.Errorf("tag %q 不能包含空格", m.tag)
	}
	// Labels go through the same parser the CLI uses, so a malformed one is
	// rejected here instead of being silently dropped there. The parser's own
	// message stays English — it is shared with the CLI, which is English by
	// convention — so the Chinese sentence around it is what the user reads
	// first. Re-implementing the check here just to translate it would be a
	// second source of truth for the same rule.
	if _, err := parseLabels(m.pushLabels(), true); err != nil {
		return fmt.Errorf("标签有误：%w", err)
	}
	return nil
}

// guiPicker is what a Browse button should select. MyGo's open dialog treats
// files and directories as mutually exclusive — OpenDialogOptions sets
// OpenFiles to !Directory and OpenDirectories to Directory — so there is no
// one dialog that accepts both, and a push source needs both.
type guiPicker int

const (
	guiPickFile guiPicker = iota
	guiPickDir
)

// browse asks the OS for a path of the given kind. The native dialog blocks,
// so it runs off the main thread and the answer comes back through
// Window.Update.
func (a *guiApp) browse(dest *string, kind guiPicker) {
	win := a.win
	title := "选择文件"
	if kind == guiPickDir {
		title = "选择目录"
	}
	go func() {
		picked, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:      win,
			Title:       title,
			Directory:   kind == guiPickDir,
			DefaultPath: *dest,
		})
		if win == nil {
			return
		}
		win.Update(func() {
			if err == nil && len(picked) > 0 {
				*dest = picked[0]
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
					ui.Text(c, "目标路径").TextColor(th.Text)
					ui.Row(c).Gap(8).Children(func() {
						ui.TextInput(c, &m.destPath).Grow(1).SingleLine()
						if ui.Button(c, "浏览…").Clicked() {
							a.browse(&m.destPath, guiPickDir)
						}
					})
					if m.passphraseShown {
						ui.Text(c, "密码").TextColor(th.Text)
						ui.TextInput(c, &m.passphrase).Password().SingleLine()
					}
				case guiOpPush:
					// A push source is either a file or a directory, and the
					// native dialog cannot offer both at once, so the two get
					// a button each. Typing the path still works for either.
					// The hint rides on the label rather than taking a row of
					// its own: at 720x420 the dialog has no vertical room to
					// spare, and the regression test enforces that.
					ui.Text(c, "源路径（文件或目录）").TextColor(th.Text)
					ui.Row(c).Gap(8).Children(func() {
						ui.TextInput(c, &m.srcPath).Grow(1).SingleLine()
						if ui.Button(c, "选择文件…").Clicked() {
							a.browse(&m.srcPath, guiPickFile)
						}
						if ui.Button(c, "选择目录…").Clicked() {
							a.browse(&m.srcPath, guiPickDir)
						}
					})

					ui.Text(c, "Tag").TextColor(th.Text)
					ui.TextInput(c, &m.tag).SingleLine()

					// Pushing unencrypted is the common case, so the key field
					// sits behind a toggle and stays empty until asked for.
					encrypt := m.passphraseShown
					if ui.Toggle(c, &encrypt, "加密").Changed() {
						m.passphraseShown = encrypt
						if !encrypt {
							m.passphrase = ""
						}
					}
					if m.passphraseShown {
						ui.Text(c, "密码").TextColor(th.Text)
						ui.TextInput(c, &m.passphrase).Password().SingleLine()
					}

					ui.Text(c, "标签（每行一个 key=value）").TextColor(th.Text)
					ui.TextArea(c, &m.labels).Lines(2, 4).FillWidth()
				case guiOpDelete:
					ui.Text(c, "这会从远程 registry 删除该 tag，且无法撤销。")
				case guiOpLabel:
					// Current labels are listed so the user edits against
					// what is actually there instead of guessing.
					if len(m.currentLabels) > 0 {
						ui.Text(c, "当前标签").TextColor(th.Text)
						ui.Column(c).FillWidth().Gap(2).Children(func() {
							for _, line := range m.currentLabels {
								ui.Text(c, line).
									TextColor(readableSecondary(th)).
									FontSize(12).
									SingleLine().
									Ellipsis("…")
							}
						})
					}
					ui.Text(c, "改动（每行一个）").TextColor(th.Text)
					ui.TextArea(c, &m.labelEdits).Lines(3, 6).FillWidth()
					// The hint rides on the label: it is a rule, not content,
					// and the dialog is already tight at 720x420.
					ui.Text(c, "key=value 设置或更新；只写 key 则移除").
						TextColor(readableSecondary(th)).FontSize(11)
				}

				if m.busy && m.kind == guiOpPull {
					a.viewProgress(c)
				}
				if m.err != nil {
					ui.Text(c, m.err.Error()).TextColor(th.Danger)
				}

				ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
					if m.busy {
						ui.Text(c, "处理中…").TextColor(readableSecondary(th))
						return
					}
					if ui.Button(c, "取消").Clicked() {
						a.closeModal()
					}
					label := m.confirm
					// A push with no source or a malformed label is rejected
					// before it starts, so the button stays live and the error
					// sits next to the field that caused it.
					ok := ui.PrimaryButton(c, label)
					if m.kind == guiOpPush && m.validatePush() != nil {
						ok = ok.Disabled(true)
					}
					if ok.Clicked() {
						m.busy = true
						m.err = nil
						switch m.kind {
						case guiOpPull:
							a.startPull()
						case guiOpPush:
							a.startPush()
						case guiOpLabel:
							a.startLabel()
						default:
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
	// The phase keys come from runPull and stay English; only the label the
	// user reads is translated, so the mapping lives here instead of being
	// pushed down into the shared pull code.
	label := ""
	switch phase {
	case "download":
		if total > 0 {
			label = fmt.Sprintf("下载中 %s / %s", formatBytes(done), formatBytes(total))
		} else {
			label = "下载中"
		}
	case "decrypt":
		label = "解密中"
	case "unpack":
		label = "解压中"
	case "check":
		label = "检查中"
	}
	if label == "" {
		return
	}
	ui.Text(c, label).TextColor(readableSecondary(th))
	if phase == "download" && total > 0 {
		ui.Progress(c, float64(done)/float64(total)).FillWidth()
	}
}
