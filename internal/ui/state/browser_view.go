package state

import (
	"diskcleaner/internal/ignore"
	"diskcleaner/internal/scan"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"strings"
)

func cells(text string) int {
	return ansi.StringWidth(text)
}

func fit(text string, width int) string {
	return ansi.Truncate(text, width, "...")
}

func tailFit(text string, width int) string {
	if cells(text) <= width {
		return text
	}
	return "..." + ansi.Cut(text, cells(text)-width+3, cells(text))
}

func pad(text string, width int, right bool) string {
	text = fit(text, width)
	spaces := strings.Repeat(" ", max(0, width-cells(text)))
	if right {
		return spaces + text
	}
	return text + spaces
}

func (m *Model) rowStyle(color string, current bool, text string) string {
	if current {
		if color == "" {
			color = "37"
		}
		// Muted slate blue (#263445) keeps the cursor visible without a vivid band.
		color += ";48;2;38;52;69"
	}
	return m.paint(color, text)
}

func (m *Model) browserFooter() []string {
	if m.Screen == "filter" {
		return []string{"Type name | Enter apply | Esc clear", "Filtering current folder only"}
	}
	actions := "Enter open | s scan | e exclusions"
	if m.Scanned {
		actions = "Space select | s rescan | e exclusions"
		if len(m.Selected) > 0 {
			actions = "Space select | p preview | d cleanup"
		}
	}
	if m.Width >= 64 {
		actions += " | <- parent | q quit"
	}
	if m.Width < 40 {
		actions = "Enter open | s scan | e exclude"
		if m.Scanned {
			actions = "Space mark | s scan | e exclude"
		}
		if m.Scanned && len(m.Selected) > 0 {
			actions = "Space mark | p preview | d clean"
		}
		return []string{actions, "? help | t sort | / find | . hide"}
	}
	if m.Width < 64 {
		return []string{actions, "v info | ? help | t sort | / | . hidden"}
	}
	return []string{actions, "v details | ? help | t sort | / filter | . hidden files"}
}

func (m *Model) renderBrowser() (string, int) {
	w, h := m.Width, m.Height
	if w < 32 || h < 12 {
		return "Resize to at least 32 x 12", 0
	}
	visible := m.visibleIndices()
	header := []string{
		m.paint(cyan, fit("DISKCLEANER | Sort: "+m.sortLabel(), w)),
		fit(fmt.Sprintf("%d items | %d selected", len(m.Entries), len(m.Selected)), w),
		fit(estimate(m.Settings.Mode, m.targets(), m.Snapshot), w),
		m.browserVisibilityStatus(),
	}
	if m.Filter != "" || m.Screen == "filter" || m.HideHidden {
		header[1] = fit(fmt.Sprintf("%d/%d shown | %d selected (%d hidden)", len(visible), len(m.Entries), len(m.Selected), m.hiddenSelections()), w)
	}
	if m.Filter != "" || m.Screen == "filter" {
		header = append(header, "Filter: "+tailFit(Safe(m.Filter), w-20)+" | Esc clear")
	}
	footer := m.browserFooter()
	for i := range footer {
		footer[i] = fit(footer[i], w)
	}
	columnHeader, rows, currentRow := m.browserRows(visible)
	if diagnostics := m.diagnostics(); len(diagnostics) > 0 {
		available := h - len(header) - len(footer) - 4
		if available > 0 {
			// Full messages remain accessible on the details screen.
			text := strings.Join(diagnostics, " | ")
			if cells(text) > w {
				text = fit(text, w-len(" | v details")) + " | v details"
			}
			header = append(header, text)
		}
	}
	// The current path sits immediately above the columns, even with a filter
	// or diagnostics, and remains fixed while the item area scrolls.
	header = append(header, m.paint("1;36", tailFit(Safe(m.Root), w)), m.paint(cyan, columnHeader))
	if len(rows) == 0 {
		message := "This folder is empty."
		if m.Filter != "" {
			message = "No matches. Esc clears the filter."
		} else if m.HideHidden && len(m.Entries) > 0 {
			message = "No visible items. . shows hidden files."
		}
		rows = []string{fit(message, w)}
	}
	return m.fixedList(header, rows, currentRow, footer)
}

func (m *Model) browserVisibilityStatus() string {
	visibility := "shown"
	if m.HideHidden {
		visibility = "hidden"
	}
	text, color := "No exclusions", red
	if len(m.Masks()) > 0 {
		text, color = fmt.Sprintf("%d exclusions", len(m.Masks())), green
	}
	return m.paint(color, fit(text+" | Hidden: "+visibility, m.Width))
}

func (m *Model) browserRows(visible []int) (string, []string, int) {
	wide := m.Width >= 64
	nameWidth := m.Width - 6
	heading := "      Name / Size / Status"
	if wide {
		nameWidth = m.Width - 42
		heading = "      " + pad("Name", nameWidth, false) + "  " + pad("Size", 10, true) + "  " + pad("Status", 12, false) + "  Usage"
	}
	var largest uint64
	for _, entry := range m.Entries {
		if bytes, known := m.sortSize(entry); m.Scanned && known {
			largest = max(largest, bytes)
		}
	}
	rows := []string{}
	currentRow := 0
	for _, index := range visible {
		entry := m.Entries[index]
		current := index == m.Cursor
		if current {
			currentRow = len(rows)
		}
		cursor, mark, color := "  ", "[ ] ", ""
		if current {
			cursor = "› "
		}
		if _, selected := m.Selected[entry.Path]; selected {
			mark, color = "[x] ", green
		}
		name := Safe(entry.Name)
		if entry.IsDir {
			name += "/"
		} else if entry.IsSymlink {
			name += "@"
		}
		amount, status, bar := "-", "Not scanned", "        "
		if ignore.Any(m.Masks(), entry.Name) {
			status, color = "Excluded", amber
		} else if m.Scanned {
			measurement, ok := m.Snapshot.Entries[entry.Path]
			status = measurement.State.String()
			if bytes, known := m.sortSize(entry); known {
				amount = size(bytes)
				filled := 0
				if largest > 0 && bytes > 0 {
					filled = max(1, int(float64(bytes)/float64(largest)*6))
				}
				bar = "[" + strings.Repeat("#", min(6, filled)) + strings.Repeat(".", 6-min(6, filled)) + "]"
				if measurement.State == scan.Partial {
					color = amber
				}
			} else {
				color = amber
				if !ok || measurement.State == scan.Unknown {
					status = "Size unknown"
				}
			}
		}
		line := cursor + mark + pad(name, nameWidth, false)
		if wide {
			line += "  " + pad(amount, 10, true) + "  " + pad(status, 12, false) + "  " + bar
			rows = append(rows, m.rowStyle(color, current, fit(line, m.Width)))
		} else {
			rows = append(rows, m.rowStyle(color, current, fit(line, m.Width)))
			meta := "      " + pad(amount, 10, true) + "  " + status
			if m.Width >= 44 {
				meta += "  " + bar
			}
			rows = append(rows, m.rowStyle(color, current, fit(meta, m.Width)))
		}
	}
	return fit(heading, m.Width), rows, currentRow
}

// Headers and footers are fixed; only the item area follows the cursor.
func (m *Model) fixedList(header, rows []string, cursor int, footer []string) (string, int) {
	available := m.Height - len(header) - len(footer) - 1
	if available < 1 {
		return "Resize to show the list", 0
	}
	span := 1
	if (m.Screen == "list" || m.Screen == "filter" || m.Screen == "") && m.Width < 64 && len(rows) > 1 {
		span = 2
	}
	start := max(0, cursor+min(span, available)-available)
	start = min(start, max(0, len(rows)-available))
	end := min(len(rows), start+available)
	result := append([]string{}, header...)
	result = append(result, rows[start:end]...)
	for len(result) < m.Height-len(footer)-1 {
		result = append(result, "")
	}
	result = append(result, fit(fmt.Sprintf("Rows %d-%d / %d", start+1, end, len(rows)), m.Width))
	result = append(result, footer...)
	return strings.Join(result, "\n"), max(0, len(rows)-available)
}

func (m *Model) renderMasks() (string, int) {
	w := m.Width
	if w < 32 || m.Height < 12 {
		return "Resize to at least 32 x 12", 0
	}
	header := []string{m.paint(cyan, "EXCLUSIONS — optional"), tailFit(Safe(m.Root), w), m.exclusionStatus(), fit("Match names at every depth, including folder contents.", w)}
	input := "Add pattern: " + tailFit(Safe(m.Input), w-13)
	header = append(header, m.rowStyle("37", m.MaskInput, fit(input, w)))
	if m.maskCursor >= 0 && m.maskCursor < len(m.maskOptions) {
		description := maskDescription(m.maskOptions[m.maskCursor])
		if m.MaskInput {
			description = "Examples: cache, *.tmp, ?ache. Names only, without /."
		}
		header = append(header, strings.Split(ansi.Wrap(description, w, ""), "\n")...)
	}
	if m.Message != "" {
		header = append(header, fit(Safe(m.Message), w))
	}
	var rows []string
	cursorRow := 0
	for i, mask := range m.maskOptions {
		cursor, mark, color := "  ", "[ ] ", ""
		if i == m.maskCursor && !m.MaskInput {
			cursor = "› "
			cursorRow = len(rows)
		}
		if m.hasMask(mask) {
			mark, color = "[x] ", amber
		}
		label := Safe(mask)
		for _, suggestion := range suggestedMasks {
			if suggestion.name == mask {
				label += " (suggested)"
			}
		}
		rows = append(rows, wrap([]string{m.rowStyle(color, i == m.maskCursor && !m.MaskInput, cursor+mark+label)}, w)...)
	}
	footer := []string{"Space/Enter toggle | a add | ? help", "Up/Down choose | Tab input | Esc back"}
	if m.MaskInput {
		footer = []string{"Enter add | Backspace edit", "Tab list | Esc cancel input"}
	}
	for i := range footer {
		footer[i] = fit(footer[i], w)
	}
	return m.fixedList(header, rows, cursorRow, footer)
}
