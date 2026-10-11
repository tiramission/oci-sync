package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"charm.land/log/v2"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
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

// selectedBarWidth is the width of the accent bar that marks the chosen
// shortcut. It is drawn on every row and only its colour changes, because a
// border takes room inside the element: a bar that appeared on selection
// alone would shove that row's text sideways and leave a gap in the column.
const selectedBarWidth float32 = 3

func (a *guiApp) viewSidebar(c *ui.Context) {
	th := c.Theme()
	// The list has to Grow and the column FillHeight, or the list takes its
	// natural height and pushes the rest of the window out of view.
	ui.Column(c).FillHeight().Width(220).Gap(6).Padding(10).Children(func() {
		// A section heading in caps-sized grey is the weakest text in the
		// window, so it gets the readable secondary colour and a size that
		// still reads as a heading rather than a caption.
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

			// Selection is an accent bar on the leading edge, not a change
			// of background. A tinted fill reads as a hover or a pressed
			// state and, worse, puts muted text on a tinted background —
			// the worst contrast pair in the window. The bar is drawn at a
			// fixed width on every row and only its colour changes: a
			// border takes room inside the element, so a border that
			// appeared only on the selected row would shift that row's
			// text sideways and every row below it.
			marker := ui.Transparent
			if r.Selected() {
				marker = th.Accent
			}

			ui.Column(c).
				FillWidth().
				Gap(2).
				BorderWidth(0, 0, 0, selectedBarWidth).
				BorderColor(marker).
				Padding(5, 6).
				Children(func() {
					// Bold takes no argument, so a conditional style has to
					// branch on the whole call: an Element is registered with
					// the build pass by the call that creates it.
					if r.Selected() {
						ui.Text(c, s.Name).Bold().TextColor(th.Accent).MaxLines(1).Ellipsis("…")
					} else {
						ui.Text(c, s.Name).MaxLines(1).Ellipsis("…")
					}
					// With the fill gone, the repo line sits on the window
					// background on every row, so one secondary colour
					// covers them all.
					ui.Text(c, s.Repo).
						TextColor(readableSecondary(th)).
						FontSize(12).
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
		ui.Text(c, fmt.Sprintf("%s — 共 %d 个 tag", head, len(rows))).TextColor(readableSecondary(th))

		// Exactly one of these takes the room the header and the action row
		// leave. Without Grow/FillHeight on the table it would claim its
		// natural height and push the buttons off the bottom of the window,
		// where they cannot be clicked.
		switch {
		case a.listErr != nil:
			ui.Text(c, "无法列出 tag："+a.listErr.Error()).TextColor(th.Danger)
			ui.Spacer(c)
		case a.loading && len(rows) == 0:
			ui.Text(c, "加载中…").TextColor(readableSecondary(th))
			ui.Spacer(c)
		case len(rows) == 0:
			ui.Text(c, "暂无 artifact。可用 'oci-sync push' 推送一个，或清除过滤条件。").
				TextColor(readableSecondary(th))
			ui.Spacer(c)
		default:
			// Chinese headers are two cells wide per character, so each
			// title needs a little more room than its English original did.
			cols := []ui.TableColumn{
				{Title: "标签", ID: "tag", Width: 240},
				{Title: "大小", ID: "size", Width: 100, Align: ui.End},
				{Title: "加密", ID: "enc", Width: 60},
				{Title: "版本", ID: "version", Width: 90},
			}
			ui.Table(c, &a.tableList, cols, len(rows), func(row, col int) {
				x := rows[row]
				switch col {
				case 0:
					ui.Text(c, x.Tag).MaxLines(1).Ellipsis("…")
				case 1:
					ui.Text(c, formatBytes(x.Size))
				case 2:
					// "是" marks an encrypted artifact, which is the one
					// cell worth noticing in a scan, so it takes the
					// readable warning colour; "否" is the absence of
					// news and stays quiet.
					if x.Encrypted {
						ui.Text(c, "是").TextColor(readableWarning(th)).Bold()
					} else {
						ui.Text(c, "否").TextColor(readableSecondary(th))
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
		// Push targets the repository rather than a row, so it stays
		// available when nothing is selected.
		if ui.Button(c, "推送…").Clicked() {
			a.openPushModal()
		}

		x, ok := a.selectedArtifact()
		if !ok {
			ui.Text(c, "请选择一个 tag 以拉取或删除。").TextColor(readableSecondary(th))
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
	})
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
