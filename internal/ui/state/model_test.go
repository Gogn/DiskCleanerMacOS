package state

import (
	"context"
	"diskcleaner/internal/cleanup"
	"diskcleaner/internal/config"
	"diskcleaner/internal/policy"
	"diskcleaner/internal/scan"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func fixture(t *testing.T) *Model {
	t.Helper()
	base, e := os.MkdirTemp(".", "ui-test-")
	if e != nil {
		t.Fatal(e)
	}
	base, _ = filepath.Abs(base)
	t.Cleanup(func() { os.RemoveAll(base) })
	root := filepath.Join(base, "root")
	os.Mkdir(root, 0700)
	os.Mkdir(filepath.Join(root, "cache"), 0700)
	os.WriteFile(filepath.Join(root, "file\n\x1b"), []byte("data"), 0600)
	home, _ := os.UserHomeDir()
	g, e := policy.NewGuard(home, filepath.Join(base, "config"), root, nil)
	if e != nil {
		t.Fatal(e)
	}
	m := New(root, config.Default(), g, filepath.Join(base, "config", "config.json"))
	if m.Message != "" {
		t.Fatal(m.Message)
	}
	return m
}
func scanNow(t *testing.T, m *Model) {
	t.Helper()
	m.Key("s")
	if m.Screen != "scan-confirm" {
		t.Fatal(m.Screen)
	}
	job := m.Key("enter")
	if job == nil {
		t.Fatal("no scan job")
	}
	m.Apply(job())
	if !m.Scanned {
		t.Fatal(m.Message)
	}
}
func TestInitialViewDoesNotDeepScanAndWarns(t *testing.T) {
	m := fixture(t)
	if m.Screen != "list" || m.Scanned || len(m.Entries) != 2 || strings.Contains(m.View(), "file\n\x1b") || !strings.Contains(m.View(), `cache/`) {
		t.Fatal(m.View())
	}
	m.Key("s")
	if !strings.Contains(m.View(), "may take a long time") || !strings.Contains(m.View(), "exclusions") {
		t.Fatal(m.View())
	}
}
func TestChangeRootClearsSelectionsAndRestoresPreviousScan(t *testing.T) {
	m := fixture(t)
	scanNow(t, m)
	old := m.Root
	m.Key("right")
	if m.Root == old || m.Scanned || len(m.Selected) != 0 {
		t.Fatal("navigation did not reset")
	}
	m.Key("left")
	if m.Root != old || !m.Scanned {
		t.Fatal("previous scan was not restored")
	}
	m.Key("r")
	m.Input = "/no/such/root"
	m.Key("enter")
	if m.Root != old {
		t.Fatal("failed navigation changed root")
	}
}
func TestManualSelectionAndExclusion(t *testing.T) {
	m := fixture(t)
	m.Key("space")
	if len(m.Selected) != 0 {
		t.Fatal("selected before scan")
	}
	scanNow(t, m)
	m.Key("space")
	if len(m.Selected) != 1 {
		t.Fatal(m.Message)
	}
	m.Key("x")
	if m.Scanned || len(m.Selected) != 0 || len(m.Masks()) != 1 {
		t.Fatal("mask did not invalidate")
	}
	saved, e := config.Load(m.ConfigPath)
	if e != nil || len(saved.Masks) != 1 {
		t.Fatal(saved, e)
	}
}
func TestPreviewOptionalAndConfirmationRequired(t *testing.T) {
	m := fixture(t)
	scanNow(t, m)
	m.Key("space")
	job := m.Key("p")
	m.Apply(job())
	if m.Screen != "preview" || len(m.Preview.Paths) != 1 {
		t.Fatal(m.Message)
	}
	m.Key("esc")
	m.Key("d")
	if m.Screen != "confirm" {
		t.Fatal(m.Screen)
	}
	m.Key("esc")
	if len(m.Selected) != 1 {
		t.Fatal("cancel cleared selection")
	}
}
func TestSelectionCapturedBeforeConfirmation(t *testing.T) {
	m := fixture(t)
	scanNow(t, m)
	m.Key("space")
	target := m.targets()[0]
	os.Rename(target.Path, target.Path+"-old")
	os.Mkdir(target.Path, 0700)
	calls := 0
	m.Remove = func(context.Context, string) error { calls++; return nil }
	m.Settings.Mode = config.Permanent
	m.Key("d")
	job := m.Key("enter")
	if m.Key("enter") != nil {
		t.Fatal("duplicate execution")
	}
	m.Apply(job())
	if calls != 0 || m.Results[0].Outcome != cleanup.Skipped || !m.Scanned || len(m.Selected) != 0 {
		t.Fatal(m.Results)
	}
}
func TestSizeLabelsFollowDeleteMode(t *testing.T) {
	m := fixture(t)
	scanNow(t, m)
	m.Key("space")
	for _, mode := range []config.DeleteMode{config.Trash, config.Permanent} {
		m.Settings.Mode = mode
		label := "Selected size"
		if mode == config.Permanent {
			label = "Estimated space to reclaim"
		}
		if !strings.Contains(m.View(), label) {
			t.Fatal(m.View())
		}
	}
}
func TestCleanupResultsKeepSnapshotAndClearSelection(t *testing.T) {
	m := fixture(t)
	scanNow(t, m)
	m.Key("space")
	m.Key("d")
	job := m.Key("enter")
	m.Key("esc")
	if !m.Busy {
		t.Fatal("did not wait")
	}
	m.Apply(job())
	if !m.Scanned || len(m.Selected) != 0 || len(m.Results) != 1 || m.Results[0].Outcome != cleanup.Skipped {
		t.Fatal(m.Results)
	}
}
func TestSettingsPersistence(t *testing.T) {
	m := fixture(t)
	m.Key("o")
	m.Key("right")
	saved, e := config.Load(m.ConfigPath)
	if e != nil || saved.Mode != config.Permanent {
		t.Fatal(saved, e)
	}
	m.Key("esc")
	m.Key("e")
	m.Input = "deep*"
	m.Key("enter")
	if len(m.Masks()) != 1 {
		t.Fatal(m.Masks())
	}
	m.ConfigPath = filepath.Join(m.Root, "file\n\x1b", "invalid")
	m.Input = "another"
	m.Key("enter")
	if !strings.Contains(m.Message, "not saved") {
		t.Fatal(m.Message)
	}
}
func TestEstimateUnknown(t *testing.T) {
	m := fixture(t)
	m.Scanned = true
	m.Snapshot = scan.Snapshot{Entries: map[string]scan.Measurement{}}
	m.Key("space")
	if !strings.Contains(m.View(), "Unknown") {
		t.Fatal(m.View())
	}
}

func TestNarrowViewWrapsPathsAndKeepsFooter(t *testing.T) {
	m := fixture(t)
	m.Width = 40
	m.Height = 24
	m.Screen = "confirm"
	m.Confirmed = []cleanup.Target{{Path: "/" + strings.Repeat("very-long-folder/", 40) + "file"}}
	view := m.View()
	for _, line := range strings.Split(view, "\n") {
		if len([]rune(plainView(line))) > 40 {
			t.Fatalf("line exceeds width: %q", line)
		}
	}
	if len(strings.Split(view, "\n")) > m.Height {
		t.Fatal("view exceeds screen")
	}
	if !strings.Contains(view, "Esc cancel") {
		t.Fatal("missing confirmation footer")
	}
	old := view
	m.Key("pgdown")
	if m.View() == old {
		t.Fatal("cannot scroll wrapped path")
	}
}

func plainView(s string) string {
	return regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(s, "")
}

func TestHelpPreservesConfirmationAndTextInput(t *testing.T) {
	m := fixture(t)
	scanNow(t, m)
	m.Key("space")
	m.Key("d")
	m.Offset = 1
	if job := m.Key("?"); job != nil || m.Screen != "help" {
		t.Fatal("help should open without starting work")
	}
	m.Key("enter")
	m.Key("pgdown")
	m.Key("esc")
	if m.Screen != "confirm" || m.Offset != 1 || len(m.Selected) != 1 || len(m.Confirmed) != 1 || m.Busy {
		t.Fatal("help changed the pending confirmation")
	}
	m.Key("esc")
	m.Key("e")
	m.Key("tab")
	m.Key("?")
	if m.Screen != "masks" || m.Input != "?" {
		t.Fatal("help intercepted a wildcard")
	}
	m.Key("esc")
	m.Key("esc")
	m.Key("s")
	job := m.Key("enter")
	m.Key("?")
	if m.Screen != "scanning" {
		t.Fatal("help interrupted ongoing work")
	}
	m.Apply(job())
}

func TestSuggestedExclusionsPersistAndInvalidateScan(t *testing.T) {
	m := fixture(t)
	scanNow(t, m)
	m.Key("space")
	m.Key("e")
	if m.Screen != "masks" || len(m.Masks()) != 0 {
		t.Fatal("suggestions should be opt-in")
	}
	m.Key("space")
	m.Key("down")
	m.Key("enter")
	if m.Scanned || len(m.Selected) != 0 || !m.hasMask("node_modules") || !m.hasMask("CloudStorage") {
		t.Fatal("suggestions did not apply through normal exclusion handling")
	}
	saved, err := config.Load(m.ConfigPath)
	if err != nil || strings.Join(saved.Masks, ",") != "node_modules,CloudStorage" {
		t.Fatal("suggestions not saved", saved, err)
	}
	m.Key("enter")
	if m.hasMask("CloudStorage") || !m.hasMask("node_modules") {
		t.Fatal("toggle removed the wrong suggestion")
	}
	m.Key("esc")
	if m.Screen != "list" {
		t.Fatal("exclusions should return to the browser")
	}
	m.Key("right")
	if len(m.Masks()) != 1 || !m.hasMask("node_modules") {
		t.Fatal("navigation lost global exclusions")
	}
}

func TestColorsWrapAndNoColorKeepsSameLayout(t *testing.T) {
	m := fixture(t)
	if !strings.Contains(m.View(), "\x1b[31mNo exclusions") {
		t.Fatal("missing red empty-exclusions status")
	}
	m.Key("e")
	m.Width, m.Height = 40, 12
	for _, key := range []string{"up", "down"} {
		m.Key(key)
		view := m.View()
		if !strings.Contains(view, m.maskOptions[m.maskCursor]) {
			t.Fatal("current suggestion is hidden", view)
		}
		for _, line := range strings.Split(view, "\n") {
			if len([]rune(plainView(line))) > m.Width {
				t.Fatal("color codes affected wrapping", line)
			}
			if strings.HasPrefix(line, "\x1b[") && !strings.HasSuffix(line, reset) {
				t.Fatal("style leaks between lines", line)
			}
		}
		m.NoColor = true
		if m.View() != plainView(view) {
			t.Fatal("disabling colors changed layout")
		}
		m.NoColor = false
	}
}
func TestReportCanBeReopened(t *testing.T) {
	m := fixture(t)
	m.Results = []cleanup.Result{{Path: "/example", Outcome: cleanup.Skipped}}
	m.Screen = "results"
	m.Key("esc")
	m.Key("l")
	if m.Screen != "results" {
		t.Fatal("report not accessible")
	}
}

func TestScrollClampsAtEnd(t *testing.T) {
	m := fixture(t)
	m.Width = 40
	m.Height = 24
	m.Screen = "confirm"
	m.Confirmed = []cleanup.Target{{Path: strings.Repeat("long/", 200)}}
	for i := 0; i < 1000; i++ {
		m.Key("down")
	}
	bottom := m.View()
	m.Key("up")
	if m.View() == bottom {
		t.Fatal("scroll offset not clamped")
	}
}
func TestUnknownEstimateHasNoNumericZero(t *testing.T) {
	m := fixture(t)
	m.Scanned = true
	m.Key("space")
	if strings.Contains(m.View(), "0.0 B") {
		t.Fatal("unknown estimate presented as zero")
	}
}

func TestMaskListCanScroll(t *testing.T) {
	m := fixture(t)
	for i := 0; i < 50; i++ {
		m.Settings.Masks = append(m.Settings.Masks, fmt.Sprintf("cache-%02d", i))
	}
	m.Key("e")
	first := m.View()
	m.Key("pgdown")
	if m.View() == first {
		t.Fatal("long exclusion list cannot scroll")
	}
}

func TestScanRefreshesListingAndPreservesCursor(t *testing.T) {
	m := fixture(t)
	m.Cursor = 1
	current, _ := m.current()
	scanNow(t, m)
	if err := os.Remove(filepath.Join(m.Root, "cache")); err != nil {
		t.Fatal(err)
	}
	added := filepath.Join(m.Root, "aaa-new")
	if err := os.WriteFile(added, []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(m.Root, "bbb-new"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(current.Path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(current.Path, 0700); err != nil {
		t.Fatal(err)
	}
	scanNow(t, m)
	if len(m.Entries) != 3 || m.Entries[0].Path != added {
		t.Fatalf("stale listing: %+v", m.Entries)
	}
	if got, ok := m.current(); !ok || got.Path != current.Path || !got.IsDir {
		t.Fatalf("cursor moved to another object: %+v", got)
	}
	if m.Snapshot.Entries[added].State != scan.Complete {
		t.Fatal("new entry has no measurement")
	}
	for _, entry := range m.Entries {
		if err := os.Remove(entry.Path); err != nil {
			t.Fatal(err)
		}
	}
	scanNow(t, m)
	if len(m.Entries) != 0 || m.Cursor != 0 {
		t.Fatal("empty listing retained stale cursor or entries")
	}
}

func TestCancelledScanKeepsPreviousListingWithoutSizes(t *testing.T) {
	m := fixture(t)
	scanNow(t, m)
	m.Key("space")
	if err := os.WriteFile(filepath.Join(m.Root, "new"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	m.Key("s")
	job := m.Key("enter")
	m.Key("esc")
	m.Apply(job())
	if m.Scanned || len(m.Selected) != 0 || len(m.Entries) != 2 || m.Screen != "list" {
		t.Fatal("cancelled scan published partial listing or retained stale sizes/selection")
	}
}

func TestListKeepsHeaderAndCurrentEntryVisible(t *testing.T) {
	m := fixture(t)
	for i := 0; i < 30; i++ {
		name := fmt.Sprintf("item-%02d-%s-END", i, strings.Repeat("long", 10))
		if err := os.WriteFile(filepath.Join(m.Root, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	m.navigate(m.Root)
	m.Width, m.Height, m.Cursor = 40, 24, 16
	m.Key("space")
	view := m.View()
	if !strings.Contains(view, "v info") || !strings.Contains(view, "Sort:") || !strings.Contains(view, "Start a scan before selecting items.") {
		t.Fatal("fixed status or details hint missing", view)
	}
	entry, _ := m.current()
	if !strings.Contains(view, "›") || !strings.Contains(view, "item-14") {
		t.Fatal("current row hidden", view)
	}
	m.Key("v")
	var details strings.Builder
	for i := 0; i < 10; i++ {
		details.WriteString(strings.ReplaceAll(m.View(), "\n", ""))
		m.Key("down")
	}
	if !strings.Contains(details.String(), Safe(entry.Path)) {
		t.Fatal("full path not available in details")
	}
	if len(strings.Split(view, "\n")) > m.Height {
		t.Fatal("view exceeds terminal height")
	}
}

func TestLongListDiagnosticsRemainAccessible(t *testing.T) {
	m := fixture(t)
	m.Width, m.Height = 40, 12
	m.Message = strings.Repeat("long diagnostic ", 100) + "MESSAGE_END"
	m.Scanned = true
	entry, _ := m.current()
	m.Snapshot = scan.Snapshot{Entries: map[string]scan.Measurement{entry.Path: {State: scan.Unknown, Err: fmt.Errorf("%sDETAIL_END", strings.Repeat("scan error ", 100))}}}
	if view := m.View(); !strings.Contains(view, "v details") || len(strings.Split(view, "\n")) > m.Height {
		t.Fatalf("message hides list or details hint:\n%s", view)
	}
	m.Key("v")
	if m.Screen != "details" {
		t.Fatal("cannot open complete item diagnostics")
	}
	var seen strings.Builder
	for i := 0; i < 200; i++ {
		seen.WriteString(strings.ReplaceAll(m.View(), "\n", ""))
		m.Key("down")
	}
	for _, want := range []string{Safe(entry.Path), "MESSAGE_END", "DETAIL_END"} {
		if !strings.Contains(seen.String(), want) {
			t.Errorf("inaccessible detail: %s", want)
		}
	}
	m.Key("esc")
	if m.Screen != "list" || m.Cursor != 0 {
		t.Fatal("details changed navigation")
	}
}
