// Package state implements the UI lifecycle independently of the terminal adapter.
package state

import (
	"context"
	"diskcleaner/internal/cleanup"
	"diskcleaner/internal/config"
	"diskcleaner/internal/explain"
	"diskcleaner/internal/ignore"
	"diskcleaner/internal/policy"
	"diskcleaner/internal/scan"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

type Job func() Event
type Event struct {
	Kind         string
	Snapshot     scan.Snapshot
	Preview      cleanup.Preview
	Results      []cleanup.Result
	Err          error
	Explanation  string
	ExplainID    uint64
	scanObserved *scanObservation
	cacheToken   *scanCacheToken
}
type Model struct {
	Root                          string
	Entries                       []scan.Entry
	Cursor, Offset, Width, Height int
	Settings                      config.Settings
	ConfigPath                    string
	Guard                         *policy.Guard
	Snapshot                      scan.Snapshot
	Scanned                       bool
	LastScanAt                    time.Time
	RestoredScan                  bool
	cacheWriteFailed              bool
	cacheInitialized              bool
	cleanupCacheLock              *os.File
	cacheEpoch                    int64
	cacheBaseline                 map[string]string
	cacheRevisions                map[string]int64
	scanCache                     map[string]cachedScan
	navigationHistory             map[string]string
	SortBySize                    bool
	Selected                      map[string]cleanup.Target
	Screen, Input, Message        string
	Busy, Quit                    bool
	NoColor                       bool
	Filter                        string
	HideHidden                    bool
	MaskInput                     bool
	maskOptions                   []string
	maskCursor                    int
	helpScreen                    string
	helpOffset                    int
	helpTopic                     int
	Progress                      scan.Progress
	Elapsed                       time.Duration
	startedAt                     time.Time
	progress                      *progressMailbox
	cancel                        context.CancelFunc
	cancelRequested               bool
	Preview                       cleanup.Preview
	Results                       []cleanup.Result
	Confirmed                     []cleanup.Target
	ConfirmedMode                 config.DeleteMode
	cleanupAttempted              []cleanup.Target
	exclusionQueue                []cleanup.Target
	exclusionDelete               bool
	Analyze                       func(context.Context, string, []string, func(scan.Progress)) scan.Snapshot
	Trash, Remove                 cleanup.Runner
	Explain                       func(context.Context, explain.Request) (string, error)
	explanation                   explanationState
}

func New(root string, settings config.Settings, g *policy.Guard, configPath string) *Model {
	if settings.Masks == nil {
		settings.Masks = []string{}
	}
	m := &Model{Settings: settings, ConfigPath: configPath, Guard: g, Screen: "list", Width: 100, Height: 30, Selected: map[string]cleanup.Target{}, Analyze: scan.AnalyzeWithProgress, Trash: cleanup.SystemTrash, Remove: cleanup.SystemRemove}
	m.Explain = explain.Run
	m.loadScanCache()
	m.navigate(root)
	return m
}
func (m *Model) Masks() []string { return m.Settings.Masks }
func (m *Model) invalidate() {
	m.scanCache = nil
	m.clearSavedScans()
	m.resetScan()
}
func (m *Model) resetScan() {
	m.Snapshot = scan.Snapshot{}
	m.Scanned = false
	m.RestoredScan = false
	m.LastScanAt = time.Time{}
	m.Selected = map[string]cleanup.Target{}
	m.sortEntries()
}
func (m *Model) navigate(p string) {
	if p == "~" || strings.HasPrefix(p, "~/") {
		h, e := os.UserHomeDir()
		if e != nil {
			m.Message = e.Error()
			return
		}
		p = filepath.Join(h, strings.TrimPrefix(strings.TrimPrefix(p, "~"), "/"))
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(m.Root, p)
	}
	root, e := filepath.EvalSymlinks(p)
	if e != nil {
		m.Message = e.Error()
		return
	}
	root, e = filepath.Abs(root)
	if e != nil {
		m.Message = e.Error()
		return
	}
	es, e := scan.List(root)
	if e != nil {
		m.Message = e.Error()
		return
	}
	if e = m.Guard.SetContext(root, m.Settings.Masks); e != nil {
		m.Message = e.Error()
		return
	}
	previousRoot := m.Root
	if previousRoot != root {
		m.rememberLocation()
	}
	m.Root = root
	m.Filter = ""
	m.Entries = es
	m.Cursor = 0
	m.Offset = 0
	m.Screen = "list"
	m.Message = ""
	m.resetScan()
	m.restoreScan()
	m.restoreLocation(previousRoot)
}

// Clipboard cleanup belongs only to explicit path input. Entry navigation uses
// navigate directly, preserving literal quotes and whitespace in file names.
func (m *Model) navigateInput() {
	input := m.Input
	path := strings.TrimSuffix(input, "\r\n")
	if path == input {
		path = strings.TrimSuffix(strings.TrimSuffix(input, "\n"), "\r")
	}
	if path == "" {
		m.Message = "Enter or paste a directory path."
		return
	}
	if strings.ContainsAny(path, "\r\n") {
		m.Message = "Enter one directory path, not multiple lines."
		return
	}
	candidates := []string{input}
	if path != input {
		candidates = append(candidates, path)
	}
	if len(path) >= 2 && ((path[0] == '\'' && path[len(path)-1] == '\'') || (path[0] == '"' && path[len(path)-1] == '"')) {
		candidates = append(candidates, path[1:len(path)-1])
	}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		m.navigate(candidate)
		if m.Message == "" {
			return
		}
	}
	m.Message = "Could not open directory: " + m.Message
}
func (m *Model) targets() []cleanup.Target {
	ts := make([]cleanup.Target, 0, len(m.Selected))
	for _, v := range m.Selected {
		ts = append(ts, v)
	}
	sort.Slice(ts, func(i, j int) bool { return ts[i].Path < ts[j].Path })
	return ts
}
func (m *Model) save() {
	if e := config.Save(m.ConfigPath, m.Settings); e != nil {
		m.Message = "Settings not saved: " + e.Error()
	} else {
		m.Message = "Settings saved."
	}
}
func (m *Model) toggleMask(mask string) {
	if e := ignore.Validate(mask); e != nil {
		m.Message = e.Error()
		return
	}
	old := m.Masks()
	next := make([]string, 0, len(old)+1)
	found := false
	for _, v := range old {
		if v == mask {
			found = true
		} else {
			next = append(next, v)
		}
	}
	if !found {
		next = append(next, mask)
	}
	if e := m.Guard.SetContext(m.Root, next); e != nil {
		m.Message = e.Error()
		return
	}
	m.Settings.Masks = next
	m.invalidate()
	m.save()
}
func (m *Model) start(kind string, work func(context.Context) Event) Job {
	ctx, c := context.WithCancel(context.Background())
	m.cancel = c
	m.cancelRequested = false
	m.Busy = true
	m.Screen = kind
	m.Message = ""
	m.Offset = 0
	return func() Event { defer c(); return work(ctx) }
}
func (m *Model) Apply(e Event) {
	if e.Kind == "explain" && (m.Screen != "ai-working" || e.ExplainID != m.explanation.id) {
		return
	}
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.Busy = false
	cancelRequested := m.cancelRequested
	m.cancelRequested = false
	m.progress = nil
	m.Offset = 0
	m.Message = ""
	switch e.Kind {
	case "explain":
		m.applyExplanation(e)
	case "scan":
		if err := m.validateScanCacheToken(e.cacheToken); err != nil {
			e.Snapshot.Err = err
		}
		m.Screen = "list"
		m.resetScan()
		if e.Snapshot.Err == errCacheConflict {
			m.restoreScan()
			m.Message = "Scan result discarded because saved sizes changed in another session; rescan."
			break
		}
		delete(m.scanCache, m.Root)
		m.persistScanCache()
		if cancelRequested && e.Snapshot.Err == nil {
			e.Snapshot.Err = context.Canceled
		}
		observed := e.scanObserved
		if e.Snapshot.Err == nil && observed != nil {
			validated, validObserved, err := observed.validate(m.Root, e.Snapshot)
			e.Snapshot, observed = validated, validObserved
			e.Snapshot.Err = err
		}
		if e.Snapshot.Err != nil {
			m.Message = "Scan failed or cancelled: " + e.Snapshot.Err.Error()
		} else {
			previous, _ := m.current()
			// Sorting the browser must not reorder the captured scan listing.
			m.Entries = append([]scan.Entry(nil), e.Snapshot.Listing...)
			m.Cursor = min(m.Cursor, max(0, len(m.Entries)-1))
			for i, entry := range m.Entries {
				if entry.Path == previous.Path {
					m.Cursor = i
					break
				}
			}
			m.Snapshot = copySnapshot(e.Snapshot)
			m.Scanned = true
			m.LastScanAt = time.Now()
			m.rememberScan(observed, e.cacheToken)
			m.sortEntries()
		}
	case "preview":
		if cancelRequested && e.Err == nil {
			e.Err = context.Canceled
		}
		if e.Err != nil {
			m.Screen = "list"
			m.Message = "Preview failed or cancelled: " + e.Err.Error()
		} else {
			m.Preview = e.Preview
			m.Screen = "preview"
		}
	case "cleanup", "cleanup-override":
		m.applyCleanupResult(e, cancelRequested)
	}
}
func (m *Model) Key(k string) Job {
	if m.Busy {
		if k == "esc" || k == "ctrl+c" {
			m.cancelRequested = true
			if m.Screen == "ai-working" {
				m.explanation.cancelled = true
			}
			if m.cancel != nil {
				m.cancel()
			}
			m.Message = "Cancelling; waiting for the operation to finish..."
		}
		return nil
	}
	if m.Screen == "exclusion-confirm" && k != "ctrl+c" {
		return m.exclusionConfirmationKey(k)
	}
	if k == "ctrl+c" {
		m.Quit = true
		return nil
	}
	if m.Screen == "filter" {
		switch k {
		case "esc":
			m.setFilter("")
			m.Screen = "list"
		case "enter":
			m.Screen = "list"
		case "backspace":
			m.setFilter(backspace(m.Filter))
		default:
			m.setFilter(m.Filter + typedCharacter(k))
		}
		return nil
	}
	if m.Screen == "help" {
		if k == "esc" || k == "?" || k == "h" {
			m.Screen, m.Offset = m.helpScreen, m.helpOffset
		} else {
			m.helpKey(k)
		}
		return nil
	}
	// In text fields, '?' remains a wildcard or part of the entered path.
	if (k == "?" || k == "h") && m.Screen != "root" && !(m.Screen == "masks" && m.MaskInput) {
		m.helpScreen, m.helpOffset = m.Screen, m.Offset
		m.helpTopic = 0
		switch m.Screen {
		case "ai-confirm", "ai-result":
			m.helpTopic = 1
		case "masks":
			m.helpTopic = 3
		case "scan-confirm":
			m.helpTopic = 2
		case "confirm", "preview", "results", "settings":
			m.helpTopic = 4
		}
		m.Screen, m.Offset = "help", 0
		return nil
	}
	if m.Screen == "ai-confirm" || m.Screen == "ai-result" {
		return m.explanationKey(k)
	}
	if k == "esc" {
		if m.Screen == "masks" && m.MaskInput {
			m.MaskInput, m.Input = false, ""
			return nil
		}
		if m.Screen == "list" {
			m.setFilter("")
		}
		m.Screen = "list"
		m.Input = ""
		m.Offset = 0
		return nil
	}
	switch m.Screen {
	case "masks":
		m.maskKey(k)
		return nil
	case "root":
		switch k {
		case "up", "down", "pgup", "pgdown":
			m.scroll(k)
		case "enter":
			m.navigateInput()
		case "ctrl+u":
			m.Input, m.Message, m.Offset = "", "", 0
		case "backspace":
			_, n := utf8.DecodeLastRuneInString(m.Input)
			if n > 0 {
				m.Input = m.Input[:len(m.Input)-n]
			}
		default:
			if k == "space" {
				m.Input += " "
			} else if utf8.RuneCountInString(k) == 1 {
				m.Input += k
			}
		}
		return nil
	case "settings":
		if k == "left" || k == "right" || k == "space" {
			if m.Settings.Mode == config.Trash {
				m.Settings.Mode = config.Permanent
			} else {
				m.Settings.Mode = config.Trash
			}
			m.save()
		}
		return nil
	case "scan-confirm":
		if k == "enter" {
			root := m.Root
			masks := append([]string(nil), m.Masks()...)
			analyze := m.Analyze
			mailbox := &progressMailbox{}
			m.progress, m.Progress, m.Elapsed = mailbox, scan.Progress{}, 0
			m.startedAt = time.Now()
			verified := m.refreshRootCache(m.Root)
			delete(m.scanCache, m.Root)
			m.persistScanCache()
			token := &scanCacheToken{verified: verified, root: root, epoch: m.cacheEpoch, revision: m.cacheRevisions[root]}
			return m.start("scanning", func(ctx context.Context) Event {
				observed, err := captureScanObservation(ctx, root)
				if err != nil {
					return Event{Kind: "scan", Snapshot: scan.Snapshot{Err: err}, cacheToken: token}
				}
				return Event{Kind: "scan", Snapshot: analyze(ctx, root, masks, mailbox.publish), scanObserved: observed, cacheToken: token}
			})
		}
		return nil
	case "confirm":
		if k == "enter" {
			ts := append([]cleanup.Target(nil), m.Confirmed...)
			mode := m.ConfirmedMode
			g := m.Guard
			m.cleanupAttempted = ts
			m.exclusionQueue = nil
			if !m.beginCleanupCache(ts) {
				m.Offset = 0
				m.Message = "Could not invalidate saved sizes; cleanup was not started. Retry when cache storage is available."
				return nil
			}
			trash, remove := m.Trash, m.Remove
			return m.start("cleaning", func(ctx context.Context) Event {
				return Event{Kind: "cleanup", Results: cleanup.Execute(ctx, mode, ts, g, trash, remove)}
			})
		}
		m.scroll(k)
		return nil
	case "preview", "results", "details":
		m.scroll(k)
		return nil
	}
	switch k {
	case ".":
		m.HideHidden = !m.HideHidden
		m.normalizeCursor()
	case "/":
		m.Screen = "filter"
	case "t":
		m.SortBySize = !m.SortBySize
		m.sortEntries()
		m.Cursor = 0
		m.normalizeCursor()
	case "q":
		m.Quit = true
	case "v":
		m.Screen = "details"
		m.Offset = 0
	case "i":
		m.openExplanation()
	case "l":
		if len(m.Results) > 0 {
			m.Screen = "results"
			m.Offset = 0
		} else {
			m.Message = "No cleanup report yet."
		}
	case "up", "k":
		m.moveCursor(-1)
	case "down", "j":
		m.moveCursor(1)
	case "pgup":
		m.moveCursor(-m.browserPageSize())
	case "pgdown":
		m.moveCursor(m.browserPageSize())
	case "home", "end":
		indices := m.visibleIndices()
		if len(indices) > 0 {
			if k == "home" {
				m.Cursor = indices[0]
			} else {
				m.Cursor = indices[len(indices)-1]
			}
		}
	case "left":
		if m.Root != "/" {
			m.navigate(filepath.Dir(m.Root))
		}
	case "right", "enter":
		if e, ok := m.current(); ok {
			m.navigate(e.Path)
		}
	case "r":
		m.Screen = "root"
		m.Input, m.Message, m.Offset = "", "", 0
	case "s":
		m.Message = ""
		m.Screen = "scan-confirm"
	case "e":
		m.openMasks()
	case "E", "shift+e":
		if e, ok := m.current(); ok {
			mask := ignore.Literal(e.Name)
			if !m.hasMask(mask) && ignore.Any(m.Masks(), e.Name) {
				m.Message = "Excluded by a pattern. Press e to edit matching patterns."
				break
			}
			m.toggleMask(mask)
			if m.Message == "Settings saved." {
				if m.hasMask(mask) {
					m.Message = fmt.Sprintf("Excluded all items named %s globally, including matching folder contents. Settings saved.", Safe(e.Name))
				} else if ignore.Any(m.Masks(), e.Name) {
					m.Message = "Name exclusion removed globally and saved; still excluded by another pattern. Press e to edit patterns."
				} else {
					m.Message = fmt.Sprintf("Removed exclusion for %s globally. Settings saved.", Safe(e.Name))
				}
			}
		}
	case "o":
		m.Screen = "settings"
	case "x":
		if e, ok := m.current(); ok && e.IsDir {
			m.toggleMask(ignore.Literal(e.Name))
			if !strings.Contains(m.Message, "not saved") {
				m.Message = fmt.Sprintf("All items named %s globally; matching directories include their contents. %s", Safe(e.Name), m.Message)
			}
		}
	case "space", " ":
		if !m.Scanned {
			m.Message = "Start a scan before selecting items."
			break
		}
		if e, ok := m.current(); ok {
			if _, yes := m.Selected[e.Path]; yes {
				delete(m.Selected, e.Path)
				break
			}
			if err := m.Guard.CheckTarget(e.Path); err != nil {
				m.Message = err.Error()
				break
			}
			target, err := cleanup.Capture(e.Path)
			if err != nil {
				m.Message = err.Error()
				break
			}
			m.Selected[e.Path] = target
			m.Message = ""
		}
	case "p":
		ts := m.targets()
		if len(ts) == 0 {
			m.Message = "Select at least one item."
			break
		}
		g, sizes := m.Guard, m.Snapshot
		return m.start("previewing", func(ctx context.Context) Event {
			p, e := cleanup.BuildPreview(ctx, ts, g, sizes)
			return Event{Kind: "preview", Preview: p, Err: e}
		})
	case "d":
		if len(m.Selected) == 0 {
			m.Message = "Select at least one item."
			break
		}
		m.Confirmed = m.targets()
		m.ConfirmedMode = m.Settings.Mode
		m.Screen = "confirm"
		m.Offset = 0
		m.Message = ""
	}
	return nil
}

func (m *Model) hasMask(mask string) bool {
	for _, existing := range m.Masks() {
		if existing == mask {
			return true
		}
	}
	return false
}
func (m *Model) current() (scan.Entry, bool) {
	if m.Cursor < 0 || m.Cursor >= len(m.Entries) || !m.matchesFilter(m.Entries[m.Cursor]) {
		return scan.Entry{}, false
	}
	return m.Entries[m.Cursor], true
}
func (m *Model) scroll(k string) {
	_, limit := m.render()
	m.Offset = min(m.Offset, limit)
	defer func() { m.Offset = min(max(0, m.Offset), limit) }()
	switch k {
	case "down", "j":
		m.Offset++
	case "up", "k":
		if m.Offset > 0 {
			m.Offset--
		}
	case "pgdown":
		m.Offset += 10
	case "pgup":
		m.Offset -= 10
		if m.Offset < 0 {
			m.Offset = 0
		}
	}
}
