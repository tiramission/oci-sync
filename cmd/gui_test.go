package cmd

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tiramission/oci-sync/internal/config"
	"github.com/tiramission/oci-sync/internal/oci"
)

// guiTestApp builds a guiApp with artifacts already loaded, so the view can be
// exercised without a registry. Nothing here calls refresh: the tests must
// not reach the network.
func guiTestApp() *guiApp {
	return newGuiApp([]config.ShortcutInfo{
		{Name: "photos", Repo: "registry.example.com/me/photos"},
		{Name: "docs", Repo: "registry.example.com/me/docs"},
	})
}

func guiWithArtifacts(a *guiApp, xs ...oci.ArtifactInfo) *guiApp {
	a.artifacts = xs
	a.repo = a.currentShortcut().Repo
	return a
}

func guiArtifact(tag string, size int64, enc bool) oci.ArtifactInfo {
	return oci.ArtifactInfo{
		Repo:      "registry.example.com/me/photos",
		Tag:       tag,
		Version:   "0.7.0",
		Size:      size,
		Encrypted: enc,
	}
}

func TestGuiVisibleKeepsEveryArtifactWithoutFilter(t *testing.T) {
	a := guiWithArtifacts(guiTestApp(), guiArtifact("daily", 10, false), guiArtifact("weekly", 20, true))

	assert.Len(t, a.guiVisible(), 2)
}

func TestGuiVisibleFiltersOnTagCaseInsensitively(t *testing.T) {
	a := guiWithArtifacts(guiTestApp(),
		guiArtifact("Daily", 10, false),
		guiArtifact("weekly", 20, true),
		guiArtifact("monthly", 30, false),
	)

	for _, needle := range []string{"dai", "DAILY", "ly"} {
		a.filter = needle
		rows := a.guiVisible()
		require.NotEmpty(t, rows, "filter %q", needle)
		for _, x := range rows {
			assert.Contains(t, strings.ToLower(x.Tag), strings.ToLower(needle),
				"filter %q let through an unrelated tag", needle)
		}
	}
}

func TestGuiVisibleWithFilterMatchingNothing(t *testing.T) {
	a := guiWithArtifacts(guiTestApp(), guiArtifact("daily", 10, false))
	a.filter = "nothing-matches-this"

	assert.Empty(t, a.guiVisible())
}

func TestSelectedArtifactFollowsTheFilteredRows(t *testing.T) {
	a := guiWithArtifacts(guiTestApp(),
		guiArtifact("daily", 10, false),
		guiArtifact("weekly", 20, true),
	)

	a.tableSel = 1
	x, ok := a.selectedArtifact()
	require.True(t, ok)
	assert.Equal(t, "weekly", x.Tag)

	// Filtering reindexes: row 1 is now out of range, so nothing is chosen
	// rather than the wrong guiArtifact.
	a.filter = "daily"
	_, ok = a.selectedArtifact()
	assert.False(t, ok, "a stale index must not select a different guiArtifact")
}

func TestSelectedArtifactOnEmptyTable(t *testing.T) {
	a := guiTestApp()

	_, ok := a.selectedArtifact()
	assert.False(t, ok)
}

func TestCurrentShortcutFallsBackWhenIndexIsOutOfRange(t *testing.T) {
	a := guiTestApp()
	a.navSel = 99

	assert.Empty(t, a.currentShortcut().Name)
}

func TestSelectShortcutResetsFilterAndSelection(t *testing.T) {
	a := guiWithArtifacts(guiTestApp(), guiArtifact("daily", 10, false))
	a.filter = "daily"
	a.tableSel = 3
	a.listErr = assert.AnError

	a.selectShortcut(1)

	assert.Equal(t, "docs", a.currentShortcut().Name)
	assert.Empty(t, a.filter)
	assert.Zero(t, a.tableSel)
	assert.NoError(t, a.listErr)
}

func TestSelectShortcutIgnoresOutOfRange(t *testing.T) {
	a := guiTestApp()
	a.navSel = 0

	a.selectShortcut(-1)
	a.selectShortcut(5)

	assert.Equal(t, "photos", a.currentShortcut().Name, "a rejected row must not switch repo")
}

func TestCloseModalClearsOpenState(t *testing.T) {
	a := guiTestApp()
	a.modalOpen = true
	a.modal = guiModal{kind: guiOpPull, tag: "daily", destPath: "./daily", passphrase: "secret"}

	a.closeModal()

	assert.False(t, a.modalOpen)
	assert.Equal(t, guiOpNone, a.modal.kind, "the passphrase must not survive into the next dialog")
	assert.Empty(t, a.modal.passphrase)
}

func TestOpenPullModalAsksForPassphraseOnlyWhenEncrypted(t *testing.T) {
	a := guiTestApp()

	a.openPullModal(guiArtifact("secret", 10, true))
	assert.True(t, a.modalOpen)
	assert.True(t, a.modal.passphraseShown)
	assert.Equal(t, "./secret", a.modal.destPath)

	a.openPullModal(guiArtifact("plain", 10, false))
	assert.False(t, a.modal.passphraseShown)
}

func TestOpenDeleteModalWarnsAndKeepsNoPassphraseField(t *testing.T) {
	a := guiTestApp()

	a.openDeleteModal(guiArtifact("daily", 10, false))

	assert.True(t, a.modalOpen)
	assert.Equal(t, guiOpDelete, a.modal.kind)
	assert.False(t, a.modal.passphraseShown)
}

func TestFinishModalKeepsTheDialogOpenForARetry(t *testing.T) {
	a := guiTestApp()
	a.openPullModal(guiArtifact("secret", 10, true))
	a.modal.busy = true

	a.finishModal(assert.AnError)

	assert.True(t, a.modalOpen, "a wrong passphrase must leave the dialog up")
	assert.False(t, a.modal.busy, "the buttons must come back so the user can retry")
	assert.Error(t, a.modal.err)
}

// Each View test builds exactly one Tester. A ListState belongs to one list
// in one window, and MyGo panics when a second Tester shows the same one —
// so "render, click, render again" has to happen inside a single tester,
// which is also how the real app behaves.
func newGuiTester(t *testing.T, a *guiApp) *ui.Tester {
	t.Helper()
	return ui.NewTester(a.view, 1000, 640)
}

func TestGuiViewRendersShortcutsAndArtifacts(t *testing.T) {
	a := guiWithArtifacts(guiTestApp(),
		guiArtifact("daily", 1024, false),
		guiArtifact("secret", 2048, true),
	)
	tt := newGuiTester(t, a)

	texts := tt.Texts()
	assert.Contains(t, texts, "photos")
	assert.Contains(t, texts, "docs")
	assert.Contains(t, texts, "daily")
	assert.Contains(t, texts, "secret")
	assert.Contains(t, texts, "是")
	assert.Contains(t, texts, "否")
}

func TestGuiViewShowsRepoAndTagCount(t *testing.T) {
	a := guiWithArtifacts(guiTestApp(), guiArtifact("daily", 1, false), guiArtifact("weekly", 1, false))
	tt := newGuiTester(t, a)

	assert.Contains(t, tt.Texts(), "registry.example.com/me/photos — 共 2 个 tag")
}

func TestGuiViewFilterHidesUnrelatedTags(t *testing.T) {
	a := guiWithArtifacts(guiTestApp(), guiArtifact("daily", 1, false), guiArtifact("weekly", 1, false))
	a.filter = "weekly"
	tt := newGuiTester(t, a)

	texts := tt.Texts()
	assert.Contains(t, texts, "weekly")
	assert.NotContains(t, texts, "daily")
	assert.Contains(t, texts, "registry.example.com/me/photos — 共 1 个 tag")
}

func TestGuiViewExplainsAnEmptyTable(t *testing.T) {
	tt := newGuiTester(t, guiTestApp())

	assert.Contains(t, tt.Texts(), "暂无 artifact。可用 'oci-sync push' 推送一个，或清除过滤条件。")
}

func TestGuiViewReportsAListFailure(t *testing.T) {
	a := guiTestApp()
	a.listErr = errors.New("registry unreachable")
	tt := newGuiTester(t, a)

	assert.Contains(t, tt.Texts(), "无法列出 tag：registry unreachable")
}

func TestGuiViewWithoutASelectionOffersNoActions(t *testing.T) {
	tt := newGuiTester(t, guiTestApp())

	assert.Contains(t, tt.Texts(), "请选择一个 tag 以拉取或删除。")
}

func TestGuiViewPullButtonOpensTheDialog(t *testing.T) {
	a := guiWithArtifacts(guiTestApp(), guiArtifact("daily", 1, false))
	a.tableSel = 0
	tt := newGuiTester(t, a)

	require.NoError(t, tt.Click("拉取…"))
	require.True(t, a.modalOpen)
	assert.Equal(t, guiOpPull, a.modal.kind)

	texts := tt.Texts()
	assert.Contains(t, texts, "拉取 daily")
	assert.Contains(t, texts, "目标路径")
	assert.NotContains(t, texts, "密码", "a plain artifact needs no key")
}

func TestGuiViewPullOfEncryptedArtifactAsksForAPassphrase(t *testing.T) {
	a := guiWithArtifacts(guiTestApp(), guiArtifact("secret", 1, true))
	a.tableSel = 0
	tt := newGuiTester(t, a)

	require.NoError(t, tt.Click("拉取…"))

	assert.Contains(t, tt.Texts(), "密码")
}

func TestGuiViewDeleteButtonOpensTheDialog(t *testing.T) {
	a := guiWithArtifacts(guiTestApp(), guiArtifact("daily", 1, false))
	a.tableSel = 0
	tt := newGuiTester(t, a)

	require.NoError(t, tt.Click("删除…"))
	require.True(t, a.modalOpen)
	assert.Equal(t, guiOpDelete, a.modal.kind)
	assert.Contains(t, tt.Texts(),
		"这会从远程 registry 删除该 tag，且无法撤销。")
}

func TestGuiViewCancelClosesTheDialog(t *testing.T) {
	a := guiWithArtifacts(guiTestApp(), guiArtifact("daily", 1, false))
	a.tableSel = 0
	tt := newGuiTester(t, a)
	require.NoError(t, tt.Click("拉取…"))
	require.True(t, a.modalOpen)

	require.NoError(t, tt.Click("取消"))
	assert.False(t, a.modalOpen)
}

func TestGuiViewShowsTheFailureAndStaysOpen(t *testing.T) {
	a := guiWithArtifacts(guiTestApp(), guiArtifact("secret", 1, true))
	a.tableSel = 0
	tt := newGuiTester(t, a)
	require.NoError(t, tt.Click("拉取…"))

	a.finishModal(errors.New("bad passphrase"))
	tt.Frame()

	assert.True(t, a.modalOpen)
	assert.Contains(t, tt.Texts(), "bad passphrase")
}

func TestGuiViewProgressNamesThePullPhase(t *testing.T) {
	a := guiWithArtifacts(guiTestApp(), guiArtifact("daily", 1, false))
	a.tableSel = 0
	tt := newGuiTester(t, a)
	require.NoError(t, tt.Click("拉取…"))

	a.modal.busy = true
	a.progress.set(PullStage{Phase: "unpack"})
	tt.Frame()

	assert.Contains(t, tt.Texts(), "解压中")
}

func TestGuiViewProgressReportsDownloadBytes(t *testing.T) {
	a := guiWithArtifacts(guiTestApp(), guiArtifact("daily", 1, false))
	a.tableSel = 0
	tt := newGuiTester(t, a)
	require.NoError(t, tt.Click("拉取…"))

	a.modal.busy = true
	a.progress.set(PullStage{Phase: "download", Done: 512, Total: 1024})
	tt.Frame()

	assert.Contains(t, tt.Texts(), "下载中 512 B / 1.0 KiB")
}

func TestGuiViewBusyPullHidesTheConfirmButton(t *testing.T) {
	a := guiWithArtifacts(guiTestApp(), guiArtifact("daily", 1, false))
	a.tableSel = 0
	tt := newGuiTester(t, a)
	require.NoError(t, tt.Click("拉取…"))

	a.modal.busy = true
	tt.Frame()

	texts := tt.Texts()
	assert.Contains(t, texts, "处理中…")
	assert.NotContains(t, texts, "取消", "a running pull must not offer a cancel that does nothing")
}

func TestGuiProgressSnapshotIsSafeWithoutAFrame(t *testing.T) {
	var p guiProgress
	p.set(PullStage{Phase: "decrypt", Done: 1, Total: 2})

	phase, done, total := p.snapshot()
	assert.Equal(t, "decrypt", phase)
	assert.EqualValues(t, 1, done)
	assert.EqualValues(t, 2, total)
}

func TestGuiViewSurvivesRepeatedFrames(t *testing.T) {
	// A build pass invalidates its elements, so anything the app kept from
	// an earlier pass would be stale. Many passes must not panic or drift.
	a := guiWithArtifacts(guiTestApp(), guiArtifact("daily", 1, false), guiArtifact("secret", 2, true))
	tt := newGuiTester(t, a)
	for range 5 {
		tt.Frame()
	}

	texts := tt.Texts()
	assert.Contains(t, texts, "daily")
	assert.Contains(t, texts, "secret")
}

func TestGuiViewStaysQuietWhenNothingIsLoading(t *testing.T) {
	a := guiWithArtifacts(guiTestApp(), guiArtifact("daily", 1, false))
	tt := newGuiTester(t, a)

	assert.NotContains(t, tt.Texts(), "加载中…")
}

func TestGuiOpenPushModalTargetsTheCurrentRepo(t *testing.T) {
	a := guiTestApp()
	a.navSel = 1

	a.openPushModal()

	require.True(t, a.modalOpen)
	assert.Equal(t, guiOpPush, a.modal.kind)
	assert.Equal(t, "docs", a.modal.repo)
	assert.Contains(t, a.modal.title, "registry.example.com/me/docs")
	assert.Equal(t, "latest", a.modal.tag)
	assert.False(t, a.modal.passphraseShown, "encrypting must be opt-in, not the default")
}

func TestGuiPushLabelsSkipBlankLines(t *testing.T) {
	m := guiModal{labels: "a=1\n\n  b=2  \n\n"}

	assert.Equal(t, []string{"a=1", "b=2"}, m.pushLabels())
}

func TestGuiPushLabelsOnEmptyInput(t *testing.T) {
	m := guiModal{}

	assert.Empty(t, m.pushLabels())
}

func TestGuiValidatePushNeedsASource(t *testing.T) {
	m := guiModal{tag: "latest", srcPath: "  "}

	assert.EqualError(t, m.validatePush(), "请选择要推送的文件或目录")
}

func TestGuiValidatePushRejectsAnEmptyTag(t *testing.T) {
	m := guiModal{srcPath: "./data", tag: "   "}

	assert.EqualError(t, m.validatePush(), "tag 不能为空")
}

func TestGuiValidatePushRejectsSpacesInATag(t *testing.T) {
	m := guiModal{srcPath: "./data", tag: "my tag"}

	assert.EqualError(t, m.validatePush(), `tag "my tag" 不能包含空格`)
}

func TestGuiValidatePushRejectsAMalformedLabel(t *testing.T) {
	m := guiModal{srcPath: "./data", tag: "latest", labels: "broken"}

	// The label parser is shared with the CLI and stays English; the GUI wraps
	// it so the sentence the user reads first is Chinese.
	assert.EqualError(t, m.validatePush(), `标签有误：invalid label format "broken", expected key=value`)
}

func TestGuiValidatePushRejectsAReservedLabel(t *testing.T) {
	m := guiModal{srcPath: "./data", tag: "latest", labels: "io.oci-sync.version=9"}

	err := m.validatePush()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reserved")
}

func TestGuiValidatePushAcceptsACompleteForm(t *testing.T) {
	m := guiModal{
		srcPath:    "./data",
		tag:        "v1.2.3",
		labels:     "env=prod\nteam=infra",
		passphrase: "secret",
	}

	assert.NoError(t, m.validatePush())
}

func TestGuiViewPushButtonIsAvailableWithNothingSelected(t *testing.T) {
	// Push targets a repository, not a row, so it must not be gated on the
	// table selection the way Pull and Delete are.
	tt := newGuiTester(t, guiTestApp())

	assert.Contains(t, tt.Texts(), "推送…")
	assert.Contains(t, tt.Texts(), "请选择一个 tag 以拉取或删除。")
}

func TestGuiViewPushButtonOpensTheDialog(t *testing.T) {
	a := guiTestApp()
	tt := newGuiTester(t, a)

	require.NoError(t, tt.Click("推送…"))
	require.True(t, a.modalOpen)
	assert.Equal(t, guiOpPush, a.modal.kind)

	texts := tt.Texts()
	assert.Contains(t, texts, "源路径")
	assert.Contains(t, texts, "Tag")
	assert.Contains(t, texts, "标签（每行一个 key=value）")
	assert.Contains(t, texts, "加密")
	assert.NotContains(t, texts, "密码", "the key field stays hidden until 加密 is on")
}

func TestGuiViewPushConfirmIsDisabledWithoutASource(t *testing.T) {
	a := guiTestApp()
	tt := newGuiTester(t, a)
	require.NoError(t, tt.Click("推送…"))

	// An incomplete form must not start a push that would fail in the
	// registry round trip.
	before := a.modal.busy
	require.NoError(t, tt.Click("推送"))
	assert.Equal(t, before, a.modal.busy, "push ran with no source selected")
}

func TestGuiViewPushCancelClosesTheDialog(t *testing.T) {
	a := guiTestApp()
	tt := newGuiTester(t, a)
	require.NoError(t, tt.Click("推送…"))

	require.NoError(t, tt.Click("取消"))
	assert.False(t, a.modalOpen)
}

func TestGuiViewDeleteDialogHasNoPushFields(t *testing.T) {
	a := guiWithArtifacts(guiTestApp(), guiArtifact("daily", 1, false))
	a.tableSel = 0
	tt := newGuiTester(t, a)
	require.NoError(t, tt.Click("删除…"))

	texts := tt.Texts()
	assert.NotContains(t, texts, "源路径")
	assert.NotContains(t, texts, "标签（每行一个 key=value）")
}

// TestGuiViewPushDialogFitsTheSmallestWindow pins the layout floor. A
// dialog that grows past the window puts its confirm button outside the
// viewport, where Click silently does nothing — the failure mode that
// Fill() caused earlier. Asserted on every field so a new one cannot be
// added without checking it still fits.
func TestGuiViewPushDialogFitsTheSmallestWindow(t *testing.T) {
	const w, h = 720, 420

	a := guiTestApp()
	tt := ui.NewTester(a.view, w, h)
	require.NoError(t, tt.Click("推送…"))
	tt.Frame()

	for _, label := range []string{
		"源路径", "Tag", "加密",
		"标签（每行一个 key=value）",
		"推送", "取消",
	} {
		r, ok := tt.Find(label)
		require.True(t, ok, "%q is missing from the push dialog", label)
		assert.Less(t, r.Y, float32(h),
			"%q starts at y=%.1f, below the %d-high window", label, r.Y, h)
		assert.GreaterOrEqual(t, r.Y+r.H, r.Y, "%q has a negative height", label)
		assert.LessOrEqual(t, r.Y+r.H, float32(h),
			"%q ends at y=%.1f, past the %d-high window", label, r.Y+r.H, h)
	}
}

// TestGuiViewActionsStayClickableAfterTheTableGrows guards the same floor
// for the main window: the action row must not be pushed off the bottom
// however many tags are listed.
func TestGuiViewActionsStayClickableAfterTheTableGrows(t *testing.T) {
	const w, h = 720, 420

	a := guiTestApp()
	for i := range 200 {
		a.artifacts = append(a.artifacts, guiArtifact(fmt.Sprintf("tag-%03d", i), 1024, false))
	}
	a.tableSel = 0
	tt := ui.NewTester(a.view, w, h)

	for _, label := range []string{"推送…", "拉取…", "删除…"} {
		r, ok := tt.Find(label)
		require.True(t, ok, "%q is missing with 200 tags listed", label)
		assert.LessOrEqual(t, r.Y+r.H, float32(h),
			"%q ends at y=%.1f, past the %d-high window", label, r.Y+r.H, h)
	}
}
