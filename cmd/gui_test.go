package cmd

import (
	"errors"
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
	assert.Contains(t, texts, "yes")
	assert.Contains(t, texts, "no")
}

func TestGuiViewShowsRepoAndTagCount(t *testing.T) {
	a := guiWithArtifacts(guiTestApp(), guiArtifact("daily", 1, false), guiArtifact("weekly", 1, false))
	tt := newGuiTester(t, a)

	assert.Contains(t, tt.Texts(), "registry.example.com/me/photos — 2 tag(s)")
}

func TestGuiViewFilterHidesUnrelatedTags(t *testing.T) {
	a := guiWithArtifacts(guiTestApp(), guiArtifact("daily", 1, false), guiArtifact("weekly", 1, false))
	a.filter = "weekly"
	tt := newGuiTester(t, a)

	texts := tt.Texts()
	assert.Contains(t, texts, "weekly")
	assert.NotContains(t, texts, "daily")
	assert.Contains(t, texts, "registry.example.com/me/photos — 1 tag(s)")
}

func TestGuiViewExplainsAnEmptyTable(t *testing.T) {
	tt := newGuiTester(t, guiTestApp())

	assert.Contains(t, tt.Texts(), "No artifacts. Push one with 'oci-sync push', or clear the filter.")
}

func TestGuiViewReportsAListFailure(t *testing.T) {
	a := guiTestApp()
	a.listErr = errors.New("registry unreachable")
	tt := newGuiTester(t, a)

	assert.Contains(t, tt.Texts(), "Cannot list tags: registry unreachable")
}

func TestGuiViewWithoutASelectionOffersNoActions(t *testing.T) {
	tt := newGuiTester(t, guiTestApp())

	assert.Contains(t, tt.Texts(), "Select a tag to pull or delete.")
}

func TestGuiViewPullButtonOpensTheDialog(t *testing.T) {
	a := guiWithArtifacts(guiTestApp(), guiArtifact("daily", 1, false))
	a.tableSel = 0
	tt := newGuiTester(t, a)

	require.NoError(t, tt.Click("Pull…"))
	require.True(t, a.modalOpen)
	assert.Equal(t, guiOpPull, a.modal.kind)

	texts := tt.Texts()
	assert.Contains(t, texts, "Pull daily")
	assert.Contains(t, texts, "Destination")
	assert.NotContains(t, texts, "Passphrase", "a plain artifact needs no key")
}

func TestGuiViewPullOfEncryptedArtifactAsksForAPassphrase(t *testing.T) {
	a := guiWithArtifacts(guiTestApp(), guiArtifact("secret", 1, true))
	a.tableSel = 0
	tt := newGuiTester(t, a)

	require.NoError(t, tt.Click("Pull…"))

	assert.Contains(t, tt.Texts(), "Passphrase")
}

func TestGuiViewDeleteButtonOpensTheDialog(t *testing.T) {
	a := guiWithArtifacts(guiTestApp(), guiArtifact("daily", 1, false))
	a.tableSel = 0
	tt := newGuiTester(t, a)

	require.NoError(t, tt.Click("Delete…"))
	require.True(t, a.modalOpen)
	assert.Equal(t, guiOpDelete, a.modal.kind)
	assert.Contains(t, tt.Texts(),
		"This removes the tag from the remote registry and cannot be undone.")
}

func TestGuiViewCancelClosesTheDialog(t *testing.T) {
	a := guiWithArtifacts(guiTestApp(), guiArtifact("daily", 1, false))
	a.tableSel = 0
	tt := newGuiTester(t, a)
	require.NoError(t, tt.Click("Pull…"))
	require.True(t, a.modalOpen)

	require.NoError(t, tt.Click("Cancel"))
	assert.False(t, a.modalOpen)
}

func TestGuiViewShowsTheFailureAndStaysOpen(t *testing.T) {
	a := guiWithArtifacts(guiTestApp(), guiArtifact("secret", 1, true))
	a.tableSel = 0
	tt := newGuiTester(t, a)
	require.NoError(t, tt.Click("Pull…"))

	a.finishModal(errors.New("bad passphrase"))
	tt.Frame()

	assert.True(t, a.modalOpen)
	assert.Contains(t, tt.Texts(), "bad passphrase")
}

func TestGuiViewProgressNamesThePullPhase(t *testing.T) {
	a := guiWithArtifacts(guiTestApp(), guiArtifact("daily", 1, false))
	a.tableSel = 0
	tt := newGuiTester(t, a)
	require.NoError(t, tt.Click("Pull…"))

	a.modal.busy = true
	a.progress.set(PullStage{Phase: "unpack"})
	tt.Frame()

	assert.Contains(t, tt.Texts(), "unpacking")
}

func TestGuiViewProgressReportsDownloadBytes(t *testing.T) {
	a := guiWithArtifacts(guiTestApp(), guiArtifact("daily", 1, false))
	a.tableSel = 0
	tt := newGuiTester(t, a)
	require.NoError(t, tt.Click("Pull…"))

	a.modal.busy = true
	a.progress.set(PullStage{Phase: "download", Done: 512, Total: 1024})
	tt.Frame()

	assert.Contains(t, tt.Texts(), "downloading 512 B / 1.0 KiB")
}

func TestGuiViewBusyPullHidesTheConfirmButton(t *testing.T) {
	a := guiWithArtifacts(guiTestApp(), guiArtifact("daily", 1, false))
	a.tableSel = 0
	tt := newGuiTester(t, a)
	require.NoError(t, tt.Click("Pull…"))

	a.modal.busy = true
	tt.Frame()

	texts := tt.Texts()
	assert.Contains(t, texts, "Working…")
	assert.NotContains(t, texts, "Cancel", "a running pull must not offer a cancel that does nothing")
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

	assert.NotContains(t, tt.Texts(), "Loading…")
}
