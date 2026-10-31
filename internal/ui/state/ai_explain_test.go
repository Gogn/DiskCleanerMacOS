package state

import (
	"context"
	"diskcleaner/internal/cleanup"
	"diskcleaner/internal/config"
	"diskcleaner/internal/explain"
	"diskcleaner/internal/scan"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestAIAnswerLanguageSavesBothChoicesAndFreezesRequest(t *testing.T) {
	m := fixture(t)
	scanNow(t, m)
	m.Key("space")
	m.Offset, m.Width = 8, 200
	before := captureExplanationBrowser(m)
	m.Key("i")
	if !strings.Contains(m.View(), "Answer language: English") {
		t.Fatal("English answer is not the default", m.View())
	}
	for i, key := range []string{"right", "left", "tab"} {
		m.Key(key)
		want, label := "ru", "Русский"
		if i == 1 {
			want, label = "en", "English"
		}
		saved, err := config.Load(m.ConfigPath)
		if err != nil || saved.ExplainLanguage != want || m.Settings.ExplainLanguage != want || !strings.Contains(m.View(), "Answer language: "+label) {
			t.Fatal("answer language switch was not saved and displayed", key, saved, err, m.View())
		}
	}
	m.Explain = func(ctx context.Context, request explain.Request) (string, error) {
		if request.Language != "ru" {
			t.Fatal("job did not capture the language at confirmation", request.Language)
		}
		return "Ответ.", nil
	}
	job := m.Key("enter")
	m.Settings.ExplainLanguage = "en"
	m.Apply(job())
	m.Key("esc")
	if !reflect.DeepEqual(captureExplanationBrowser(m), before) {
		t.Fatal("changing answer language modified scan, selection or navigation state")
	}
}

func TestAIAnswerLanguageSaveFailureKeepsChoiceAndDiagnostic(t *testing.T) {
	m := fixture(t)
	blocked := filepath.Join(filepath.Dir(m.Root), "not-a-directory")
	if err := os.WriteFile(blocked, nil, 0600); err != nil {
		t.Fatal(err)
	}
	m.ConfigPath = filepath.Join(blocked, "config.json")
	m.Key("i")
	m.Key("right")
	if m.Settings.ExplainLanguage != "ru" || !strings.Contains(m.View(), "Answer language: Русский") || !strings.Contains(m.Message, "Settings not saved:") {
		t.Fatal("failed save lost active answer choice or error", m.View())
	}
	m.Key("esc")
	if m.Settings.ExplainLanguage != "ru" || !strings.Contains(m.Message, "Settings not saved:") {
		t.Fatal("returning hid the save failure or discarded the active choice")
	}
}

func TestAIAnswerLanguageDefaultForManualModelAndVisibleBeforeLongPath(t *testing.T) {
	m := &Model{Root: "/", Screen: "list", Width: 32, Height: 12, Entries: []scan.Entry{{Path: "/" + strings.Repeat("long-path/", 30), Name: "long item"}}, Settings: config.Settings{}}
	m.Explain = func(ctx context.Context, request explain.Request) (string, error) {
		if request.Language != "en" {
			t.Fatal("empty manual settings did not request English", request.Language)
		}
		return "Explanation", nil
	}
	m.Key("i")
	lines := strings.Split(plainView(m.View()), "\n")
	if len(lines) > m.Height || !strings.Contains(lines[2], "Answer language: English") {
		t.Fatal("language choice is hidden behind a long path", m.View())
	}
	for _, line := range lines {
		if cells(line) > m.Width {
			t.Fatal("language confirmation exceeds narrow terminal width", line)
		}
	}
	m.Apply(m.Key("enter")())
	if m.Screen != "ai-result" {
		t.Fatal("manual settings could not request an explanation", m.Message)
	}
}

type explanationBrowserState struct {
	root, filter                    string
	cursor, offset                  int
	scanned, hideHidden, sortBySize bool
	lastScanAt                      time.Time
	entries                         []scan.Entry
	snapshot                        scan.Snapshot
	selected                        map[string]cleanup.Target
	cache                           map[string]cachedScan
}

func captureExplanationBrowser(m *Model) explanationBrowserState {
	s := explanationBrowserState{root: m.Root, filter: m.Filter, cursor: m.Cursor, offset: m.Offset, scanned: m.Scanned, hideHidden: m.HideHidden, sortBySize: m.SortBySize, lastScanAt: m.LastScanAt, entries: append([]scan.Entry(nil), m.Entries...), snapshot: copySnapshot(m.Snapshot), selected: map[string]cleanup.Target{}, cache: map[string]cachedScan{}}
	for path, target := range m.Selected {
		s.selected[path] = target
	}
	for root, cached := range m.scanCache {
		cached.snapshot = copySnapshot(cached.snapshot)
		s.cache[root] = cached
	}
	return s
}

func TestAIExplanationConfirmsHighlightedItemAndPreservesBrowserState(t *testing.T) {
	m := fixture(t)
	scanNow(t, m)
	m.Key("space")
	m.setFilter("file")
	m.Offset, m.Message, m.NoColor, m.Width = 7, "Previous browser notification.", true, 200
	entry, _ := m.current()
	before := captureExplanationBrowser(m)
	calls := 0
	m.Explain = func(ctx context.Context, request explain.Request) (string, error) {
		calls++
		if request.Path != entry.Path {
			t.Fatal("AI request used a selected item instead of the highlighted path", request.Path)
		}
		return "First line\nSecond line\nUntrusted: \x1b[31m\tend", nil
	}
	if job := m.Key("i"); job != nil || calls != 0 || m.Screen != "ai-confirm" {
		t.Fatal("AI explanation started before confirmation")
	}
	view := m.View()
	for _, want := range []string{Safe(entry.Path), "Sends path, name, type, sizes including the last scan result"} {
		if !strings.Contains(view, want) {
			t.Fatal("confirmation omitted captured path or disclosure", want, view)
		}
	}
	for _, want := range []string{"may read file contents", "send them to the AI service", "outside this item", "read-only command permissions", "Enter ask Codex"} {
		if !strings.Contains(strings.ReplaceAll(view, "\n", ""), want) {
			t.Fatal("confirmation omitted Codex read access or launch hint", want, view)
		}
	}
	if strings.Contains(view, "Previous browser notification.") {
		t.Fatal("confirmation showed a stale notification")
	}
	m.Key("?")
	m.Key("esc")
	m.Key("esc")
	if m.Screen != "list" || m.Message != "Previous browser notification." || !reflect.DeepEqual(captureExplanationBrowser(m), before) {
		t.Fatal("back via help did not restore the browser")
	}
	m.Key("i")
	job := m.Key("enter")
	if job == nil || !m.Busy || calls != 0 || strings.Contains(m.View(), "Lines ") {
		t.Fatal("AI job was not asynchronous or busy screen offered scrolling")
	}
	m.Apply(job())
	if m.Screen != "ai-result" || m.Busy || calls != 1 {
		t.Fatal("explanation result not shown")
	}
	view = m.View()
	for _, want := range []string{"First line\nSecond line", `Untrusted: \x1b[31m\tend`, "AI inference may be wrong.", "No files are selected or deleted automatically."} {
		if !strings.Contains(view, want) {
			t.Fatal("result lost line breaks, escaping or inference notice", want, view)
		}
	}
	if strings.Contains(view, "\x1b") || strings.Contains(view, "Lines ") {
		t.Fatal("short result contains a control code or unnecessary scroll indicator", view)
	}
	m.Key("esc")
	if m.Screen != "list" || !reflect.DeepEqual(captureExplanationBrowser(m), before) {
		t.Fatal("AI result changed browser state")
	}
}

func TestAIExplanationCancellationDiscardsLateOutputAndStaleEvents(t *testing.T) {
	m := fixture(t)
	scanNow(t, m)
	m.Key("space")
	m.Offset = 9
	before := captureExplanationBrowser(m)
	started := make(chan struct{})
	m.Explain = func(ctx context.Context, request explain.Request) (string, error) {
		close(started)
		<-ctx.Done()
		return "LATE OUTPUT MUST NOT APPEAR", nil
	}
	m.Key("i")
	job := m.Key("enter")
	done := make(chan Event, 1)
	go func() { done <- job() }()
	<-started
	m.Key("esc")
	if !m.Busy || m.Screen != "ai-working" || !strings.Contains(m.View(), "Cancelling") {
		t.Fatal("cancellation did not wait for the job")
	}
	event := <-done
	m.Apply(event)
	if m.Busy || m.Screen != "list" || strings.Contains(m.View(), "LATE OUTPUT") || !reflect.DeepEqual(captureExplanationBrowser(m), before) {
		t.Fatal("cancelled explanation published output or changed browser state")
	}
	m.Explain = func(context.Context, explain.Request) (string, error) { return "Current explanation", nil }
	m.Key("i")
	next := m.Key("enter")
	m.Apply(event)
	if !m.Busy || m.Screen != "ai-working" {
		t.Fatal("old completion interrupted a newer request")
	}
	m.Apply(next())
	if m.Screen != "ai-result" || !strings.Contains(m.View(), "Current explanation") {
		t.Fatal("new request failed after stale completion")
	}
}

func TestAIExplanationErrorsAndNoCurrentItem(t *testing.T) {
	m := fixture(t)
	m.Offset = 4
	before := captureExplanationBrowser(m)
	m.Explain = func(context.Context, explain.Request) (string, error) { return "", errors.New("request timed out") }
	m.Key("i")
	m.Apply(m.Key("enter")())
	if m.Screen != "list" || !strings.Contains(m.Message, "request timed out") || !reflect.DeepEqual(captureExplanationBrowser(m), before) {
		t.Fatal("AI error lost browser state or its diagnostic")
	}
	m.setFilter("no matching item")
	m.Key("i")
	if m.Screen != "list" || !strings.Contains(m.Message, "Highlight an item") {
		t.Fatal("empty current item opened confirmation")
	}
	m.Key("/")
	m.Key("i")
	if m.Screen != "filter" || m.Filter != "no matching itemi" {
		t.Fatal("i was not typed literally in filter input")
	}
}

func TestAIExplanationConfirmationAndResultsScrollOnlyOnOverflow(t *testing.T) {
	m := fixture(t)
	m.Width, m.Height = 32, 12
	m.Explain = func(context.Context, explain.Request) (string, error) {
		return strings.Repeat("Explanation line\n", 50) + "RESULT END", nil
	}
	m.Key("i")
	if _, limit := m.render(); limit == 0 {
		t.Fatal("long confirmation cannot scroll")
	}
	m.Key("end")
	if m.Offset == 0 || !strings.Contains(strings.ReplaceAll(m.View(), "\n", ""), "files outside this item.") {
		t.Fatal("full metadata disclosure is inaccessible", m.View())
	}
	m.Key("home")
	if m.Offset != 0 {
		t.Fatal("confirmation Home did not return to start")
	}
	m.Apply(m.Key("enter")())
	if _, limit := m.render(); limit == 0 {
		t.Fatal("long result cannot scroll")
	}
	m.Key("end")
	if !strings.Contains(m.View(), "RESULT END") {
		t.Fatal("result end is inaccessible", m.View())
	}
	m.Key("home")
	if m.Offset != 0 {
		t.Fatal("result Home did not return to start")
	}
}

func TestAIExplanationRequestCarriesLastScanAndDeleteMode(t *testing.T) {
	m := fixture(t)
	var got explain.Request
	m.Explain = func(ctx context.Context, request explain.Request) (string, error) {
		got = request
		return "Answer.", nil
	}
	ask := func(name string) explain.Request {
		t.Helper()
		m.Key("home")
		for entry, ok := m.current(); ok && entry.Name != name; entry, ok = m.current() {
			m.Key("down")
		}
		m.Key("i")
		m.Apply(m.Key("enter")())
		m.Key("esc")
		if filepath.Base(got.Path) != name {
			t.Fatal("explained the wrong item", got.Path)
		}
		return got
	}
	if r := ask("cache"); r.LastScan != nil || r.DeleteMode != "trash" {
		t.Fatalf("unscanned item claimed a scan result or lost the deletion mode: %+v", r)
	}
	scanNow(t, m)
	file := ask("file\n\x1b")
	want := m.Snapshot.Entries[file.Path]
	if file.LastScan == nil || file.LastScan.State != "complete" || file.LastScan.AllocatedBytes == nil || uint64(*file.LastScan.AllocatedBytes) != want.Bytes || !file.LastScan.ScannedAt.Equal(m.LastScanAt) {
		t.Fatalf("scan measurement was not passed to the explanation: %+v", file.LastScan)
	}
	m.Key("home")
	m.Key("E")
	scanNow(t, m)
	excluded := ask("cache")
	if excluded.LastScan == nil || excluded.LastScan.State != "skipped" || excluded.LastScan.AllocatedBytes != nil || excluded.LastScan.SkipReason != "excluded by a name pattern" {
		t.Fatalf("excluded item was not reported as skipped by a pattern: %+v", excluded.LastScan)
	}
}
