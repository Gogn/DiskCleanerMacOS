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
		"2. CHOOSE EXCLUSIONS (OPTIONAL)", "Press e to review suggestions or add a pattern. All suggestions start unchecked unless previously saved.",
		"3. SCAN", "Press s, then Enter. Wait for sizes; Esc cancels.",
		"4. SELECT", "Move to an item and press Space: [ ] becomes [x]. Selecting alone does not delete anything.",
		"5. REVIEW AND CLEAN", "p opens an optional dry run. d shows the exact selected paths and mode; Enter confirms, Esc cancels.",
		"HELP TOPICS", "1 Quick start   2 Browsing   3 Scanning", "4 Exclusions    5 Cleanup    6 Display & keys",
	}},
	{"Browsing", []string{
		"NAVIGATION", "Up/Down or k/j  Move through visible items", "Enter / Right  Open the highlighted folder", "Left           Go to the parent folder", "r              Enter an absolute path, ~, or a relative path", "v              Show the full path and diagnostics",
		"SORTING", "t switches Alphabetical (default) and Size (largest first). Ties use name order; unknown/excluded sizes go last. Sorting keeps the same highlighted item and selections.",
		"FILTERING", "/ starts a case-insensitive name filter in the current folder. Enter applies it; Esc clears it. Filtering does not scan or change selections.",
		"HIDDEN FILES", ". toggles dot-prefixed files and folders. They are shown by default. This changes the list only: hidden files still participate in scans and cleanup.",
		"HIDDEN SELECTIONS", "Items hidden by a filter or the dot-file toggle stay selected. Their count stays visible, and cleanup confirmation includes them.",
		"Changing folders clears the scan, selections, and text filter. The hidden-file preference and sort mode last for this session.",
	}},
	{"Scanning & sizes", []string{
		"START / CANCEL", "s, then Enter  Scan all immediate items in this folder", "Esc / Ctrl+C   Cancel ongoing work and wait for it to finish",
		"PROGRESS", "Current path, elapsed time, and processed/total item counts are shown. Skipped items count as processed. One large folder can take longer than many small files, so there is no percentage or ETA.",
		"SIZE LABELS", "Complete      Allocated disk space measured successfully", "Partial       Available size; some contents could not be measured", "Size unknown  No reliable measurement, not a zero-byte item", "Excluded      Skipped by a name pattern", "Skipped       Not measured, for example on a different volume",
		"USAGE BARS", "Bars compare an item with the largest measured item in the folder, not the whole disk. Partial bars remain estimates. Unknown and excluded items have no bar.",
		"Scans do not follow symbolic links or cross into another volume. Open a volume directly to scan it.",
		"Sizes and selections clear after changing folders, changing exclusions, or attempting cleanup. Start another scan when needed.",
	}},
	{"Exclusions", []string{
		"CHECKBOX LIST", "e             Open saved patterns and suggestions", "Up/Down       Choose a pattern (PgUp/PgDn moves faster)", "Space / Enter Enable or disable the highlighted pattern", "a / Tab       Focus Add pattern / switch input and list", "Enter (input) Add the typed pattern", "Esc           Cancel input first, then close the editor",
		"WHAT GETS EXCLUDED?", "Patterns match names at every depth in the current root. Matching folders include their entire contents. Rules save automatically for this root and are not inherited when opening another folder.",
		"PATTERN EXAMPLES", "node_modules  Exact name, including nested matches", "*.tmp         Any name ending in .tmp", "?ache         One character followed by ache", "Use a backslash to escape a literal * or ?. Paths containing / are not patterns.",
		"SUGGESTIONS", "Dependencies: node_modules, .venv, venv, vendor", "Cloud and history: CloudStorage, .git", "Caches: __pycache__, .cache, Caches, .gradle", "Build output: .next, .build, DerivedData, target, dist, build",
		"Nothing is enabled automatically. Read the description under the highlighted pattern. Broad names such as build match every occurrence, including unrelated folders.",
		"Exclusions skip analysis and block cleanup of matching objects. A selected parent containing an excluded object is also blocked. Disable a pattern and rescan if you intend to inspect or clean those items.",
		"Changing exclusions clears sizes and selections. Unchecked custom patterns remain available until the editor closes.",
	}},
	{"Cleanup", []string{
		"SELECT AND REVIEW", "Space  Toggle an item after scanning: [ ] / [x]", "p      Optional dry run; no files are changed", "d      Review all selected paths and the deletion mode", "Enter  Execute once, only from the confirmation screen", "Esc    Close confirmation and keep your selection", "l      Reopen the last cleanup report",
		"DELETION MODE", "o opens settings; Left/Right or Space switches Trash (default) and Permanent. Permanent deletion cannot be undone. A Trash failure never falls back to permanent deletion.",
		"SELECTION SCOPE", "Filtering and hiding dot-files never remove selections. Hidden selected paths are explicitly noted and included in confirmation. Selected folders include ordinary files added before the operation.",
		"PROTECTION", "Excluded or protected contents block cleanup, including through a parent folder. Paths are checked again before acting. Avoid cleaning folders that other processes are changing.",
		"REPORT", "Completed  Operation finished successfully", "Skipped    Operation was not started; the reason is shown", "Failed     Changes may have occurred; check the report",
		"Esc / Ctrl+C during cleanup stops further work and cancels the current command. It cannot undo changes already made. Wait for the final report.",
	}},
	{"Display & keys", []string{
		"CURSOR VS SELECTION", "Muted blue background / › marker: current row", "[ ]: not selected for cleanup", "[x]: selected for cleanup; in exclusions, pattern enabled", "Moving the cursor never changes a checkbox.",
		"COLORS", "Cyan: headings and the current folder path", "Green: selected items or completed actions", "Yellow: exclusions, partial or unknown sizes", "Red: no exclusions (optional), failures, or permanent deletion", "Warning colors stay visible under the cursor. NO_COLOR=1 disables colors without changing the controls.",
		"LAYOUT", "The folder path stays directly above the file columns. Status and action hints remain fixed while items scroll. Below 64 columns, each item uses two rows. Minimum browser size: 32 x 12.",
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
	available := m.Height - len(header) - len(footer) - 1
	limit := max(0, len(body)-available)
	start := min(max(0, m.Offset), limit)
	end := min(len(body), start+available)
	result := append(header, body[start:end]...)
	for len(result) < m.Height-len(footer)-1 {
		result = append(result, "")
	}
	result = append(result, fit(fmt.Sprintf("Lines %d-%d / %d | PgUp/PgDn scroll", start+1, end, len(body)), m.Width))
	result = append(result, footer...)
	return strings.Join(result, "\n"), limit
}
