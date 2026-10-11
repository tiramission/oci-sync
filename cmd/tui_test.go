package cmd

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/tiramission/oci-sync/internal/config"
	"github.com/tiramission/oci-sync/internal/oci"
)

func TestNewTuiCmd(t *testing.T) {
	cmd := newTuiCmd()
	if cmd.Use != "tui" {
		t.Errorf("expected command use to be 'tui', got %q", cmd.Use)
	}
	if cmd.Short == "" {
		t.Error("expected short description to be set")
	}
	if cmd.RunE == nil {
		t.Error("expected RunE function to be set")
	}
}

func newTestModel() model {
	m := newTuiModel(context.Background(), []config.ShortcutInfo{
		{Name: "alpha", Repo: "registry.example.com/alpha"},
		{Name: "beta", Repo: "registry.example.com/beta"},
	})
	m.width, m.height = 100, 30
	m.help.Width = 96
	return m
}

func withArtifacts(m model) model {
	m.repo = "registry.example.com/alpha"
	m.artifacts = []oci.ArtifactInfo{
		{Repo: m.repo, Tag: "v1", FullName: m.repo + ":v1", Size: 1024, Version: "0.1.0"},
		{Repo: m.repo, Tag: "fix-linux", FullName: m.repo + ":fix-linux", Size: 512, Encrypted: true, Version: "0.1.0"},
		{Repo: m.repo, Tag: "LATEST", FullName: m.repo + ":LATEST", Size: 2048, Version: "0.2.0"},
	}
	m.applyFilter()
	m.focusPanes = true
	m.shortcutIdx = 0
	return m
}

func press(t *testing.T, m model, msgs ...tea.Msg) model {
	t.Helper()
	for _, msg := range msgs {
		updated, _ := m.Update(msg)
		m = updated.(model)
	}
	return m
}

func keys(s string) tea.Msg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

var (
	enterMsg = tea.Msg(tea.KeyMsg{Type: tea.KeyEnter})
	escMsg   = tea.Msg(tea.KeyMsg{Type: tea.KeyEsc})
	ctrlCMsg = tea.Msg(tea.KeyMsg{Type: tea.KeyCtrlC})
)

func TestVimNavigationMatchesConvention(t *testing.T) {
	m := press(t, newTestModel(), keys("j"))
	if m.shortcutIdx != 1 {
		t.Fatalf("j should move down, got idx %d", m.shortcutIdx)
	}
	m = press(t, m, keys("k"))
	if m.shortcutIdx != 0 {
		t.Fatalf("k should move up, got idx %d", m.shortcutIdx)
	}
	m = press(t, m, keys("k"))
	if m.shortcutIdx != 0 {
		t.Fatalf("navigation should clamp at the top, got idx %d", m.shortcutIdx)
	}
}

func TestQuitKeysReturnQuitCmd(t *testing.T) {
	m := newTestModel()
	for _, msg := range []tea.Msg{keys("q"), ctrlCMsg} {
		_, cmd := m.Update(msg)
		if cmd == nil {
			t.Fatalf("%v did not return a command", msg)
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Fatalf("%v did not produce tea.QuitMsg", msg)
		}
	}
}

func TestFilterIsSmartCase(t *testing.T) {
	m := withArtifacts(newTestModel())
	m.setFilter("fix")
	if len(m.visible) != 1 || m.visible[0].Tag != "fix-linux" {
		t.Fatalf("lowercase query should match case-insensitively, got %v", m.visible)
	}
	m.setFilter("LATEST")
	if len(m.visible) != 1 || m.visible[0].Tag != "LATEST" {
		t.Fatalf("mixed-case query should be case-sensitive, got %v", m.visible)
	}
	m.setFilter("")
	if len(m.visible) != 3 {
		t.Fatalf("empty filter should show all artifacts, got %d", len(m.visible))
	}
}

func TestEnterOpensDetailAndEscCloses(t *testing.T) {
	m := press(t, withArtifacts(newTestModel()), enterMsg)
	if m.state != stateDetail {
		t.Fatalf("enter on an artifact should open details, got state %d", m.state)
	}
	m = press(t, m, escMsg)
	if m.state != stateBrowse {
		t.Fatalf("esc should close details, got state %d", m.state)
	}
}

func TestDeleteConfirmationDefaultsToNo(t *testing.T) {
	m := press(t, withArtifacts(newTestModel()), keys("d"))
	if m.state != stateConfirmDelete {
		t.Fatalf("d should open confirmation, got state %d", m.state)
	}
	if m.confirmYes {
		t.Fatal("confirmation must default to No")
	}
	m = press(t, m, enterMsg)
	if m.op != opNone || m.state != stateBrowse {
		t.Fatalf("enter on the default No must not delete, got op %d state %d", m.op, m.state)
	}
}

func TestDeleteConfirmationYesStartsOperation(t *testing.T) {
	m := press(t, withArtifacts(newTestModel()), keys("d"), keys("y"))
	if !m.confirmYes {
		t.Fatal("y should select Yes")
	}
	m = press(t, m, enterMsg)
	if m.op != opDelete {
		t.Fatalf("confirming Yes should start the delete, got op %d", m.op)
	}
	if m.opTarget != "alpha:v1" {
		t.Fatalf("unexpected delete target %q", m.opTarget)
	}
}

func TestEncryptedPullAsksForPassphrase(t *testing.T) {
	m := withArtifacts(newTestModel())
	m.artifactIdx = 1 // fix-linux, encrypted
	m = press(t, m, keys("p"), enterMsg)
	if m.state != stateInputPassphrase {
		t.Fatalf("encrypted artifact should require a passphrase, got state %d", m.state)
	}
	m = press(t, m, escMsg)
	if m.state != stateInputPath || m.input.Value() != "./fix-linux" {
		t.Fatalf("esc should return to path prompt with value restored, got state %d value %q", m.state, m.input.Value())
	}
}

func TestTextEntryOwnsNavigationKeys(t *testing.T) {
	m := press(t, withArtifacts(newTestModel()), keys("/"))
	if m.state != stateFilter {
		t.Fatalf("/ should open the filter, got state %d", m.state)
	}
	m = press(t, m, keys("v1"))
	if m.input.Value() != "v1" {
		t.Fatalf("keys must reach the filter input, got %q", m.input.Value())
	}
	if len(m.visible) != 1 {
		t.Fatalf("filter should live-update the table, got %d matches", len(m.visible))
	}
	m = press(t, m, escMsg)
	if m.filterText != "" || m.state != stateBrowse {
		t.Fatalf("esc should clear the filter and browse, got text %q state %d", m.filterText, m.state)
	}
}

func TestCanceledPullShowsCanceledResult(t *testing.T) {
	m := press(t, withArtifacts(newTestModel()), keys("p"), enterMsg)
	if m.op != opPull {
		t.Fatalf("pull should be in flight, got op %d", m.op)
	}
	m = press(t, m, escMsg) // esc requests cancellation
	m = press(t, m, pullResultMsg{err: context.Canceled})
	if m.state != stateResult || !strings.Contains(m.resultTitle, "canceled") {
		t.Fatalf("canceled pull should show a canceled result, got state %d title %q", m.state, m.resultTitle)
	}
}

func TestDeleteSuccessDismissRefetches(t *testing.T) {
	m := press(t, withArtifacts(newTestModel()))
	m = m.showResult(opDelete, nil)
	if m.state != stateResult || !m.resultOK {
		t.Fatalf("successful delete should show a success result, got state %d ok %v", m.state, m.resultOK)
	}
	m = press(t, m, enterMsg)
	if m.op != opFetch {
		t.Fatalf("dismissing a successful delete should trigger a refetch, got op %d", m.op)
	}
}

func TestTruncateUsesCellWidth(t *testing.T) {
	got := truncate("你好世界", 5)
	if runewidth.StringWidth(got) > 5 {
		t.Fatalf("truncate must measure cells, got %q width %d", got, runewidth.StringWidth(got))
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("truncated value should end with an ellipsis, got %q", got)
	}
}

func TestRenderedLinesFitEverySupportedSize(t *testing.T) {
	bases := []func() model{newTestModel, func() model { return withArtifacts(newTestModel()) }}
	sizes := [][2]int{{48, 12}, {60, 24}, {76, 24}, {80, 24}, {120, 40}, {220, 60}}
	for _, base := range bases {
		for _, size := range sizes {
			m := press(t, base(), tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			views := map[string]string{
				"browse":    m.View(),
				"detail":    press(t, m, enterMsg).View(),
				"confirm":   press(t, m, keys("d")).View(),
				"pullpath":  press(t, m, keys("p")).View(),
				"result":    m.showResult(opPull, nil).View(),
				"help":      press(t, m, keys("?")).View(),
				"filtering": press(t, m, keys("/"), keys("v1")).View(),
				"pushform":  press(t, m, keys("P")).View(),
				"editing":   press(t, m, keys("e")).View(),
			}
			for name, view := range views {
				if line := firstOverWideLine(view, size[0]); line != "" {
					t.Fatalf("size %dx%d state %q has a %d-cell line: %q", size[0], size[1], name, lipgloss.Width(line), line)
				}
			}
		}
	}
}

func TestTooSmallFloorMessage(t *testing.T) {
	m := press(t, newTestModel(), tea.WindowSizeMsg{Width: 40, Height: 10})
	if view := m.View(); !strings.Contains(view, "terminal too small") {
		t.Fatalf("below the minimum size the UI must state the requirement, got %q", view)
	}
}

func TestPullStartsSpinnerTickLoop(t *testing.T) {
	m := withArtifacts(newTestModel())
	m = press(t, m, keys("p"))
	_, cmd := m.Update(enterMsg)
	if cmd == nil {
		t.Fatal("submitting the pull path should return a batched command")
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok || len(batch) < 2 {
		t.Fatalf("pull must batch the work cmd with a spinner tick to drive redraws, got %T", cmd())
	}
}

func TestPullOverlayRendersDownloadProgress(t *testing.T) {
	m := press(t, withArtifacts(newTestModel()), tea.WindowSizeMsg{Width: 100, Height: 30}, keys("p"), enterMsg)
	if m.op != opPull || m.pullStage == nil {
		t.Fatalf("expected an in-flight pull with a progress sink, got op %d", m.op)
	}
	m.pullStage.set(PullStage{Phase: "download", Done: 5 << 20, Total: 10 << 20})
	view := m.View()
	for _, want := range []string{"50%", "█", "MiB"} {
		if !strings.Contains(view, want) {
			t.Fatalf("busy overlay must show download progress (missing %q):\n%s", want, view)
		}
	}
	m.pullStage.set(PullStage{Phase: "decrypt"})
	if view := m.View(); !strings.Contains(view, "Decrypting") {
		t.Fatalf("overlay must show the decrypt phase:\n%s", view)
	}
}

func TestRenderBarCellCount(t *testing.T) {
	bar := renderBar(1, 3, 20)
	if n := len([]rune(bar)); n != 20 {
		t.Fatalf("renderBar must fill exactly w cells, got %d: %q", n, bar)
	}
	if full := renderBar(3, 3, 10); full != strings.Repeat("█", 10) {
		t.Fatalf("completed bar must be full blocks, got %q", full)
	}
}

// Focus is carried by the border's weight, not its colour: the focused pane
// is drawn with a double rule. A terminal cannot do the design's half-pixel
// border, and weight is the cue that survives a glance.
func TestFocusedPaneIsDrawnWithADoubleRule(t *testing.T) {
	m := withArtifacts(newTestModel())
	m = press(t, m, tea.KeyMsg{Type: tea.KeyTab})

	view := m.View()
	if !strings.Contains(view, "┃") {
		t.Fatalf("expected a double rule on the focused pane:\n%s", view)
	}
}

// An overlay owns the keyboard, so neither pane may claim focus while it is
// up — otherwise the frame behind a dialog advertises a focus the user does
// not have.
func TestNeitherPaneClaimsFocusUnderAnOverlay(t *testing.T) {
	m := press(t, withArtifacts(newTestModel()), tea.KeyMsg{Type: tea.KeyTab}, keys("P"))

	view := m.View()
	if strings.Contains(view, "┃") || strings.Contains(view, "┏") {
		t.Fatalf("no pane should be focused while the push form is open:\n%s", view)
	}
}

// The footer lists the keys a first-time user needs. At 80 columns there is
// not room for all of them, and a footer that silently shed half its entries
// teaches nobody which key went missing — so it ends in a pointer to the
// full table instead.
func TestFooterPointsAtTheKeyTableWhenItHasToDropKeys(t *testing.T) {
	m := newTestModel()
	m.width = 60
	m.help.Width = 56

	view := m.View()
	if !strings.Contains(view, "more") {
		t.Fatalf("expected the footer to point at the key table:\n%s", view)
	}
}

// A footer that fits keeps every binding and drops the pointer, so the
// hint never appears on a window wide enough not to need it.
func TestFooterOmitsThePointerWhenEverythingFits(t *testing.T) {
	view := withArtifacts(newTestModel()).View()
	if strings.Contains(view, "more") {
		t.Fatalf("a 100 column footer should list every key:\n%s", view)
	}
}

// Push and pull sit one keystroke apart and both act on the current
// repository, so binding them to the same key would make a muscle-memory
// slip write to the registry instead of reading from it.
func TestPushAndPullUseDistinctKeys(t *testing.T) {
	m := withArtifacts(newTestModel())
	m = press(t, m, keys("P"))
	if m.state != statePush {
		t.Fatalf("shift+p should open the push dialog, got state %d", m.state)
	}
	m = press(t, m, escMsg)
	m = press(t, m, keys("p"))
	if m.state != stateInputPath {
		t.Fatalf("lowercase p should open the pull path prompt, got state %d", m.state)
	}
}

func TestPushFormRequiresSourceAndTag(t *testing.T) {
	m := press(t, withArtifacts(newTestModel()), keys("P"))
	m = press(t, m, enterMsg) // tag is prefilled, source is not
	if m.state != statePush {
		t.Fatalf("an incomplete form must stay open, got state %d", m.state)
	}
	if m.op != opNone {
		t.Fatalf("nothing may start before the form validates, got op %d", m.op)
	}
	if m.push.err == "" {
		t.Fatal("the missing field must be reported in the dialog")
	}
}

func TestPushFormTabMovesBetweenFieldsAndKeepsValues(t *testing.T) {
	m := press(t, withArtifacts(newTestModel()), keys("P"), keys("/tmp/build"))
	if m.push.fields[pushFieldSource].value != "/tmp/build" {
		t.Fatalf("typing must reach the source field, got %q", m.push.fields[pushFieldSource].value)
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.push.focus != pushFieldTag {
		t.Fatalf("tab should advance to the tag field, got %d", m.push.focus)
	}
	if m.push.fields[pushFieldTag].value == "" {
		t.Fatal("the prefilled tag should still be there after tabbing away and back")
	}
}

func TestPushFormRejectsWhitespaceInTag(t *testing.T) {
	f := newPushForm()
	f.fields[pushFieldSource].value = "/tmp/build"
	f.fields[pushFieldTag].value = "my tag"
	if err := f.validate(); err == "" {
		t.Fatal("a tag containing whitespace must be rejected")
	}
	f.fields[pushFieldTag].value = "v1.0.0"
	f.fields[pushFieldLabels].value = "env=prod,broken"
	if err := f.validate(); err == "" {
		t.Fatal("a malformed label must be rejected rather than silently dropped")
	}
	f.fields[pushFieldLabels].value = "env=prod build=1"
	if err := f.validate(); err != "" {
		t.Fatalf("well-formed labels must pass, got %v", err)
	}
}

func TestPushStartsWithSubmittedForm(t *testing.T) {
	m := press(t, withArtifacts(newTestModel()), keys("P"), keys("/tmp/build"))
	m = press(t, m, tea.KeyMsg{Type: tea.KeyTab})
	m = press(t, m, tea.KeyMsg{Type: tea.KeyCtrlU}) // clear the prefilled date
	m = press(t, m, keys("v1.0.0"), enterMsg)
	if m.op != opPush {
		t.Fatalf("a valid form should start the push, got op %d", m.op)
	}
	if m.opTarget != "alpha:v1.0.0" {
		t.Fatalf("unexpected push target %q", m.opTarget)
	}
}

func TestPushFormMasksPassphraseWhenNotFocused(t *testing.T) {
	m := press(t, withArtifacts(newTestModel()), keys("P"))
	m.push.fields[pushFieldPassphrase].value = "hunter2"
	m = press(t, m, tea.KeyMsg{Type: tea.KeyTab})
	view := m.View()
	if strings.Contains(view, "hunter2") {
		t.Fatalf("the passphrase must not be on screen once it loses focus:\n%s", view)
	}
}

func TestPushEscCancelsWithoutStarting(t *testing.T) {
	m := press(t, withArtifacts(newTestModel()), keys("P"), keys("/tmp/build"), escMsg)
	if m.state != stateBrowse || m.op != opNone {
		t.Fatalf("esc must close the dialog without pushing, got state %d op %d", m.state, m.op)
	}
}

func TestEditLabelsSplitsSetFromUnset(t *testing.T) {
	m := press(t, withArtifacts(newTestModel()), keys("e"))
	if m.state != stateEditLabels {
		t.Fatalf("e should open the label editor, got state %d", m.state)
	}
	m = press(t, m, keys("env=prod,stale"), enterMsg)
	if m.op != opLabel {
		t.Fatalf("submitting labels should start the update, got op %d", m.op)
	}
	if m.opTarget != "alpha:v1" {
		t.Fatalf("unexpected label target %q", m.opTarget)
	}
}

func TestEditLabelsEmptyInputDoesNothing(t *testing.T) {
	m := press(t, withArtifacts(newTestModel()), keys("e"), enterMsg)
	if m.op != opNone || m.state != stateBrowse {
		t.Fatalf("an empty editor must not call the registry, got op %d state %d", m.op, m.state)
	}
}

// An empty repository and an over-narrow filter look identical in the table
// but need opposite actions, so the copy has to tell them apart.
func TestEmptyRepositoryAndEmptyFilterAreDistinct(t *testing.T) {
	m := withArtifacts(newTestModel())
	m.artifacts = nil
	m.applyFilter()
	view := m.View()
	if !strings.Contains(view, "no artifacts yet") || !strings.Contains(view, "P pushes") {
		t.Fatalf("an empty repository should offer the push action:\n%s", view)
	}
	m2 := withArtifacts(newTestModel())
	m2.setFilter("zzz")
	view2 := m2.View()
	if !strings.Contains(view2, "No tags match") {
		t.Fatalf("a filtered-empty table must say the filter is the cause:\n%s", view2)
	}
	if strings.Contains(view2, "no artifacts yet") {
		t.Fatalf("a filtered-empty table must not claim the repository is empty:\n%s", view2)
	}
}

func firstOverWideLine(view string, maxW int) string {
	for _, line := range strings.Split(view, "\n") {
		if lipgloss.Width(line) > maxW {
			return line
		}
	}
	return ""
}
