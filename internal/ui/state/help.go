package state

import (
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"strings"
)

var helpTopics = []struct {
	title string
	lines []string
}{
	{"Quick start", []string{
		"1. BROWSE", "Open a folder with Enter. Nothing is scanned automatically.",
		"2. CHOOSE EXCLUSIONS (OPTIONAL)", "Press e to edit the global list. Changes apply when scanning any selected folder or disk.",
		"3. SCAN", "Press s, then Enter. Wait for sizes; Esc cancels.",
		"4. SELECT", "Move to an item and press Space: [ ] becomes [x]. Selecting alone does not delete anything.",
		"5. REVIEW AND CLEAN", "p opens an optional dry run. d shows the exact selected paths and mode; Enter confirms, Esc cancels.",
		"HELP TOPICS", "1 Quick start   2 Browsing   3 Scanning", "4 Exclusions    5 Cleanup    6 Display & keys",
	}},
	{"Browsing", []string{
		"NAVIGATION", "Up/Down or k/j  Move one visible item", "PgUp/PgDn       Move one visible viewport", "Home/End        First/last visible item", "Enter / Right  Open the highlighted folder", "Left           Go to the parent folder", "r              Open an empty path field", "v              Show the full path and diagnostics",
		"PASTE A PATH", "Press r for an empty path field, then paste/type an absolute, ~ / ~/ or current-folder-relative directory path and press Enter. The r path shortcut stays in the browser footer. Matching outer quotes and one trailing copied newline are accepted; spaces and Unicode work. Ctrl+U clears input. Multiline or invalid paths are rejected; an invalid path leaves this folder and browser state unchanged. Path entry does not change filters or exclusions.",
		"SORTING", "t switches Alphabetical (default) and Size (largest first), moving the cursor to the first visible item. Selections and the current scan stay intact. Ties use name order; unknown/excluded sizes go last. Automatic sorting keeps the established cursor history.",
		"FILTERING", "/ starts a case-insensitive name filter in the current folder. Enter applies it; Esc clears it. Filtering does not scan or change selections.",
		"HIDDEN FILES", ". toggles dot-prefixed files and folders. They are shown by default. This changes the list only: hidden files still participate in scans and cleanup.",
		"HIDDEN SELECTIONS", "Items hidden by a filter or the dot-file toggle stay selected. Their count stays visible, and cleanup confirmation includes them.",
		"AI EXPLAIN", "i explains the highlighted item; no selection or prior scan is needed, but a scanned size is included when available. Before Enter, review that Codex may read and send file contents, including files outside this path. Answer language defaults to English; Left/Right or Tab selects Русский and saves the choice. This changes only the answer; instructions remain in English. Enter starts; Esc backs out or cancels and waits, then returns. DiskCleaner keeps the selection and never starts cleanup automatically. Errors show diagnostics.",
		"Changing folders clears selections and the text filter. Each visited folder remembers its highlighted path; missing or hidden items fall back to a visible item. Going up highlights the folder just left, even on the parent's first visit. Failed navigation preserves the current folder and history. History lasts until exit.",
	}},
	{"Scanning & sizes", []string{
		"START / CANCEL", "s, then Enter  Measure each immediate item, including nested contents", "Esc / Ctrl+C   Cancel ongoing work and wait for it to finish, even for an empty scan",
		"PROGRESS", "Current path, elapsed time, and processed/total item counts are shown. Skipped items count as processed. One large folder can take longer than many small files, so there is no percentage or ETA.",
		"SIZE LABELS", "Complete      Allocated disk space measured successfully", "Partial       Available size; some contents could not be measured", "Size unknown  No reliable measurement, not a zero-byte item", "Excluded      Skipped by a name pattern", "Skipped       Not measured, for example on a different volume",
		"USAGE BARS", "Bars compare an item with the largest measured item in the folder, not the whole disk. Partial bars remain estimates. Unknown and excluded items have no bar.",
		"Scans do not follow symbolic links or cross into another volume. Open a volume directly to scan it.",
		"s opens confirmation and clears previous browser notifications, then Enter refreshes this folder. Entries that disappear during listing are skipped; other listing errors fail the scan. Cancellation is honored through final result application. New errors and cancellation messages remain visible. Each folder size includes nested contents; open and scan it to list its children separately.",
		"Last scan is the time of the last successful measurement, not a live check of the tree. For restored results, the header omits its separate Last scan row; the red warning and v details retain the time. The restored Complete label in the Status column is red; rows otherwise keep normal colors and [x] still marks selection. Unknown/partial counts include all current entries, even filtered or hidden ones; excluded and skipped entries are omitted.",
		"Successful scans are saved between launches in the private SQLite database scan-cache.sqlite, which stores paths, metadata and sizes but no file contents. Restored sizes may be stale. The red warning includes the last successful scan time; v details shows the full warning: Saved sizes may have changed; last scan YYYY-MM-DD HH:MM; press s to rescan. This is measurement time, not the time files were last checked. The warning and restored Complete status are red; ordinary row colors are unchanged.",
		"Saved sizes may be temporarily unavailable to other app instances while cleanup runs. If saved sizes cannot be invalidated safely, cleanup does not start; retry when storage is available.",
		"Before applying or restoring results, immediate-entry metadata is checked; only matching paths, types, identities, sizes and modification times reuse measurements. A replaced root is rejected. Nested changes may escape these checks.",
		"Changing global exclusions clears all saved sizes and scan times. Cleanup invalidates sizes for affected targets and ancestor directories, and evicts scans rooted inside affected folders; unrelated sibling sizes and roots remain. The listing refreshes without rescanning, and selections clear. A failed or cancelled rescan removes only this folder's saved results and never publishes partial results.",
	}},
	{"Exclusions", []string{
		"QUICK EXCLUSION", "E (Shift+E) adds the highlighted file or folder name to exclusions without opening the editor. Repeating E removes that name exclusion. If another pattern still excludes it, press e to edit patterns. x toggles a folder name exclusion.",
		"CHECKBOX LIST", "e             Open saved patterns and suggestions", "Up/Down       Choose a pattern (PgUp/PgDn moves faster)", "Space / Enter Enable or disable the highlighted pattern", "a / Tab       Focus Add pattern / switch input and list", "Enter (input) Add the typed pattern", "Esc           Cancel input first, then close the editor",
		"WHAT GETS EXCLUDED?", "The same global list matches descendant names below any chosen scan root. Matching folders encountered there include their contents. The root's own name and its ancestors are not matched: after entering an excluded folder directly, a manual scan measures its children. Editing anywhere changes the shared list; there are no per-folder exclusion lists or saved overrides. A one-time folder cleanup override requires separate confirmation and does not change this list.",
		"PATTERN EXAMPLES", "node_modules  Exact name, including nested matches", "*.tmp         Any name ending in .tmp", "?ache         One character followed by ache", "Use a backslash to escape a literal * or ?. Paths containing / are not patterns.",
		"SUGGESTIONS", "Dependencies: node_modules, .venv, venv, vendor", "Cloud and history: CloudStorage, .git", "Caches: __pycache__, .cache, Caches, .gradle", "Build output: .next, .build, DerivedData, target, dist, build",
		"Nothing is enabled automatically. Read the description under the highlighted pattern. Broad names such as build match every occurrence, including unrelated folders.",
		"Matching descendants are skipped during analysis and blocked from cleanup, including through a selected parent. These name rules do not forbid entering a matching folder directly. Built-in system protection still applies. Disable a pattern and rescan to inspect or clean matched items.",
		"Changes save automatically and clear all cached sizes, scan times and selections. If saving fails, rules still apply this session but may be lost after exit. Older per-folder lists are merged when loaded; the next normal save writes the global list. Unchecked custom patterns stay until the editor closes.",
	}},
	{"Cleanup", []string{
		"SELECT AND REVIEW", "Space  Toggle an item after scanning: [ ] / [x]", "p      Optional dry run; no files are changed", "d      Review all selected paths and the deletion mode", "Enter  Execute once, only from the confirmation screen", "Esc    Close confirmation and keep your selection", "l      Reopen the last cleanup report",
		"DELETION MODE", "o opens settings; Left/Right or Space switches Trash (default) and Permanent. Permanent deletion cannot be undone. A Trash failure never falls back to permanent deletion.",
		"SELECTION SCOPE", "Filtering and hiding dot-files never remove selections. Hidden selected paths are explicitly noted and included in confirmation. Selected folders include ordinary files added before the operation.",
		"PROTECTION", "Protected contents always block cleanup, and paths are checked again before acting. If a selected folder contains excluded descendants, cleanup first leaves that folder untouched and offers a separate choice: leave it untouched or retry this folder including all excluded contents. Left selects leave; Right/Tab selects retry; Enter confirms for this folder; Esc leaves all pending folders untouched and returns to the report. Retry uses the same deletion mode and identity check; completed items do not run again. This choice applies only to this folder and is not saved. Avoid cleaning folders that other processes are changing.",
		"REPORT", "Completed  Operation finished successfully", "Skipped    Operation was not started; the reason is shown", "Failed     Changes may have occurred; check the report",
		"Esc / Ctrl+C during cleanup stops further work and cancels the current command. It cannot undo changes already made. Wait for the final report.",
	}},
	{"Display & keys", []string{
		"CURSOR VS SELECTION", "Muted blue background / › marker: current row", "[ ]: not selected for cleanup", "[x]: selected for cleanup; in exclusions, pattern enabled", "Moving the cursor never changes a checkbox.",
		"COLORS", "Cyan: headings and the current folder path", "Green: selected items or completed actions", "Yellow: exclusions, partial or unknown sizes", "Red: no exclusions (optional), failures, or permanent deletion", "Warning colors stay visible under the cursor. NO_COLOR=1 disables colors without changing the controls.",
		"LAYOUT", "The compact header omits a separate title and 0 selected. The highlighted path stays directly above the columns. Selection, hidden-selection warnings, estimates, filters, diagnostics and exclusions take priority; optional sort and scan-time rows may be omitted. Below 64 columns each entry uses two lines; 32 x 12 still fits one complete entry.",
		"The browser has two footer lines: stable navigation for the window width, then actions for the current scan/selection state. Commands are whole key/action labels; lower-priority hints are omitted instead of clipped. Selected items keep preview and cleanup hints. Filter input shows Enter apply and Esc clear; ? is literal there. v shows full paths, diagnostics and scan status.",
		"Lines appears only when text overflows on scrollable help, details, preview, cleanup confirmation/report, AI explanation confirmation/results or path input. A narrow screen may show More. Scan/working screens (including AI explanation requests), scan confirmation and settings have no text-scroll indicator. Browser and exclusion lists keep their separate Rows counter.",
		"Long names and paths are shortened; v shows them in full. Names have no surrounding quotation marks. Special characters remain escaped, such as \\n for a newline, to keep the terminal safe.",
		"HELP AND EXIT", "? / h          Open help (typed literally in text fields)", "Left/Right/Tab Switch help topics; 1-6 jumps directly", "Up/Down/PgUp/PgDn scroll; Home/End go to top/bottom", "Esc / ? / h    Return to the previous screen", "q              Quit from the browser", "Ctrl+C         Cancel work; otherwise quit",
	}},
}

func (m *Model) helpKey(key string) {
	switch key {
	case "left":
		m.helpTopic = (m.helpTopic + len(helpTopics) - 1) % len(helpTopics)
		m.Offset = 0
	case "right", "tab":
		m.helpTopic = (m.helpTopic + 1) % len(helpTopics)
		m.Offset = 0
	case "1", "2", "3", "4", "5", "6":
		m.helpTopic = int(key[0] - '1')
		m.Offset = 0
	case "home":
		m.Offset = 0
	case "end":
		_, m.Offset = m.renderHelp()
	default:
		m.scroll(key)
	}
}

func (m *Model) renderHelp() (string, int) {
	if m.Width < 32 || m.Height < 12 {
		return "Resize to at least 32 x 12", 0
	}
	topic := helpTopics[m.helpTopic]
	header := []string{m.paint(cyan, "DISKCLEANER HELP"), m.paint("1;36", fit(fmt.Sprintf("%d/%d  %s", m.helpTopic+1, len(helpTopics), topic.title), m.Width))}
	var body []string
	for _, line := range topic.lines {
		heading := line == strings.ToUpper(line) && !strings.Contains(line, "  ")
		if heading {
			if len(body) > 0 {
				body = append(body, "")
			}
		}
		for _, part := range strings.Split(ansi.Wrap(line, m.Width, ""), "\n") {
			if heading {
				part = m.paint(cyan, part)
			}
			body = append(body, part)
		}
	}
	footer := []string{"Left/Right/Tab topics | 1-6 jump", "Up/Down scroll | Esc / ? back"}
	available := m.Height - len(header) - len(footer)
	showIndicator := len(body) > available
	if showIndicator {
		available--
	}
	limit := max(0, len(body)-available)
	start := min(max(0, m.Offset), limit)
	end := min(len(body), start+available)
	result := append(header, body[start:end]...)
	reserved := len(footer)
	if showIndicator {
		reserved++
	}
	for len(result) < m.Height-reserved {
		result = append(result, "")
	}
	if showIndicator {
		result = append(result, fit(fmt.Sprintf("Lines %d-%d / %d | PgUp/PgDn scroll", start+1, end, len(body)), m.Width))
	}
	result = append(result, footer...)
	return strings.Join(result, "\n"), limit
}
