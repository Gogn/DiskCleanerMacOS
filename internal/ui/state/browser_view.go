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

// Commands are whole tokens: a narrow footer never cuts a key or its label.
func commandLine(width int, commands ...string) string {
	line := ""
	for _, command := range commands {
		candidate := command
		if line != "" {
			candidate = line + " | " + command
		}
		if cells(candidate) <= width {
			line = candidate
		}
	}
	return line
}

func (m *Model) browserFooter() []string {
	if m.Screen == "filter" {
		return []string{"Enter apply | Esc clear", "Type to filter this folder"}
	}
	nav := "Enter open | ← parent | ? help"
	if m.Width >= 64 {
		nav = commandLine(m.Width, "↑↓ move", "Enter open", "← parent", "? help", "PgUp/PgDn page", "Home/End", "q quit")
	} else if m.Width >= 40 {
		nav = commandLine(m.Width, "Enter open", "← parent", "? help", "q quit")
	}
	mark, exclude, clean, details := "Space mark", "e exclude", "d clean", "v info"
	if m.Width >= 64 {
		mark, exclude, clean, details = "Space select", "e exclusions", "d cleanup", "v details"
	}
	actions := []string{"s scan", exclude, details, "t sort", "/ filter", ". hidden", "E exclude"}
	if m.Scanned {
		actions = []string{mark, "s rescan", details, exclude, "t sort", "/ filter", ". hidden", "E exclude"}
		if len(m.Selected) > 0 {
			actions = []string{mark, "p preview", clean, details, exclude, "t sort", "/ filter", ". hidden", "E exclude"}
		}
	}
	return []string{nav, commandLine(m.Width, actions...)}
}

type browserLayout struct {
	header, rows, footer []string
	cursor               int
}

func (m *Model) browserLayout() browserLayout {
	w := m.Width
	visible := m.visibleIndices()
	columns, rows, cursor := m.browserRows(visible)
	footer := m.browserFooter()
	span := 1
	if w < 64 {
		span = 2
	}
	// Reserve a complete entry, the path, columns, row counter and both footers.
	budget := m.Height - len(footer) - 3 - span
	count := fmt.Sprintf("%d items", len(m.Entries))
	if m.Filter != "" || m.Screen == "filter" || m.HideHidden {
		count = fmt.Sprintf("%d/%d shown", len(visible), len(m.Entries))
	}
	primary := count
	if len(m.Selected) > 0 {
		primary += fmt.Sprintf(" | %d selected", len(m.Selected))
	}
	if hidden := m.hiddenSelections(); hidden > 0 {
		primary += fmt.Sprintf(" | %d hidden", hidden)
		if cells(primary) > w {
			primary = fmt.Sprintf("%d selected | %d hidden", len(m.Selected), hidden)
		}
	}
	primary = fit(primary, w)
	required := []string{}
	if len(m.Selected) > 0 {
		summary := estimate(m.Settings.Mode, m.targets(), m.Snapshot)
		if cells(summary) > w {
			summary = strings.Replace(summary, "Estimated space to reclaim: ", "Est. reclaim: ", 1)
			summary = strings.Replace(summary, "Selected size: ", "Selected: ", 1)
			summary = strings.Replace(summary, "Size unknown · Unknown", "Size unknown", 1)
		}
		required = append(required, fit(summary, w))
	}
	filter := ""
	if m.Filter != "" || m.Screen == "filter" {
		filter = "Filter: " + tailFit(Safe(m.Filter), w-8)
		required = append(required, filter)
	}
	diagnostic := ""
	if diagnostics := m.diagnostics(); len(diagnostics) > 0 {
		diagnostic = strings.Join(diagnostics, " | ")
		if cells(diagnostic) > w {
			diagnostic = fit(diagnostic, w-len(" | v details")) + " | v details"
		}
		required = append(required, diagnostic)
	}
	// Keep metadata in the same row when it fits; otherwise prefer complete
	// compact tokens over squeezing an unreadable status into the header.
	sortText := "Sort: Name"
	if m.SortBySize {
		sortText = "Sort: " + m.sortLabel()
	}
	exclusions := "No exclusions"
	if len(m.Masks()) > 0 {
		exclusions = fmt.Sprintf("%d exclusions", len(m.Masks()))
		exclusions = m.paint(green, exclusions)
	} else {
		exclusions = m.paint(red, exclusions)
	}
	visibility := "Hidden shown"
	if m.HideHidden {
		visibility = "Hidden off"
	}
	meta := []string{exclusions, visibility, sortText}
	extra := []string{}
	for _, token := range meta {
		if cells(primary+" | "+token) <= w {
			primary += " | " + token
			continue
		}
		if len(extra) > 0 && cells(extra[len(extra)-1]+" | "+token) <= w {
			extra[len(extra)-1] += " | " + token
		} else {
			extra = append(extra, token)
		}
	}
	room := max(0, budget-1-len(required))
	if len(extra) > room {
		extra = extra[:room]
	}
	header := []string{primary}
	for _, line := range extra {
		header = append(header, line)
	}
	if m.Scanned && !m.LastScanAt.IsZero() && len(header)+len(required) < budget {
		// Full date and completeness remain available in item details.
		status := m.scanStatus(false)
		if cells(status) > w {
			status = "Scan " + m.LastScanAt.Local().Format("15:04") + " | Incomplete"
		}
		header = append(header, status)
	}
	header = append(header, required...)
	header = append(header, m.paint("1;36", tailFit(Safe(m.Root), w)), m.paint(cyan, columns))
	if len(rows) == 0 {
		message := "This folder is empty."
		if m.Filter != "" {
			message = "No matches. Esc clears the filter."
		} else if m.HideHidden && len(m.Entries) > 0 {
			message = "No visible items. . shows hidden files."
		}
		rows = []string{fit(message, w)}
	}
	return browserLayout{header: header, rows: rows, footer: footer, cursor: cursor}
}

func (m *Model) browserPageSize() int {
	if m.Width < 32 || m.Height < 12 {
		return 1
	}
	layout := m.browserLayout()
	span := 1
	if m.Width < 64 {
		span = 2
	}
	return max(1, m.listRowCapacity(layout.header, layout.footer, span)/span)
}

func (m *Model) renderBrowser() (string, int) {
	if m.Width < 32 || m.Height < 12 {
		return "Resize to at least 32 x 12", 0
	}
	layout := m.browserLayout()
	return m.fixedList(layout.header, layout.rows, layout.cursor, layout.footer)
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

func (m *Model) scanStatus(full bool) string {
	unknown, partial := 0, 0
	for _, entry := range m.Entries {
		if ignore.Any(m.Masks(), entry.Name) {
			continue
		}
		measurement, ok := m.Snapshot.Entries[entry.Path]
		if !ok {
			unknown++
			continue
		}
		switch measurement.State {
		case scan.Unknown, scan.Pending:
			unknown++
		case scan.Partial:
			partial++
		case scan.Skipped:
			// Skipped entries are intentionally omitted from completeness counts.
		}
	}
	if full {
		text := "Last scan: " + m.LastScanAt.Local().Format("2006-01-02 15:04:05 MST")
		if unknown == 0 && partial == 0 {
			return text + " | Complete"
		}
		if unknown > 0 {
			text += fmt.Sprintf(" | %d unknown", unknown)
		}
		if partial > 0 {
			text += fmt.Sprintf(" | %d partial", partial)
		}
		return text
	}
	text := "Last scan " + m.LastScanAt.Local().Format("15:04")
	if m.Width < 64 {
		text = "Scan " + m.LastScanAt.Local().Format("15:04")
	}
	if unknown == 0 && partial == 0 {
		return text + " | Complete"
	}
	if unknown > 0 {
		text += fmt.Sprintf(" | %d unknown", unknown)
	}
	if partial > 0 {
		text += fmt.Sprintf(" | %d partial", partial)
	}
	return text
}

// Headers and footers are fixed; only the item area follows the cursor.
func (m *Model) listRowCapacity(header, footer []string, span int) int {
	available := m.Height - len(header) - len(footer) - 1
	return max(0, available/span*span)
}

func (m *Model) fixedList(header, rows []string, cursor int, footer []string) (string, int) {
	span := 1
	if (m.Screen == "list" || m.Screen == "filter" || m.Screen == "") && m.Width < 64 && len(rows) > 1 {
		span = 2
	}
	available := m.listRowCapacity(header, footer, span)
	if available < span {
		return "Resize to show the list", 0
	}
	start := max(0, cursor+min(span, available)-available)
	start = (start + span - 1) / span * span
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
	header := []string{m.paint(cyan, "EXCLUSIONS — GLOBAL"), tailFit(Safe(m.Root), w), m.exclusionStatus(), fit("Applies to every folder and disk; matches names at any depth.", w)}
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
