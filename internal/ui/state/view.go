package state

import (
	"diskcleaner/internal/cleanup"
	"diskcleaner/internal/config"
	"diskcleaner/internal/scan"
	"fmt"
	"strconv"
	"strings"
)

const (
	red   = "31"
	green = "32"
	amber = "33"
	cyan  = "36"
	reset = "\x1b[0m"
)

// Style whole logical lines; wrap reapplies the style to each physical line so
// scrolling cannot lose the opening color or leak it into another UI element.
func (m *Model) paint(color, text string) string {
	if m.NoColor || color == "" {
		return text
	}
	return "\x1b[" + color + "m" + text + reset
}

func (m *Model) exclusionStatus() string {
	if len(m.Masks()) == 0 {
		return m.paint(red, fit("No exclusions (optional)", m.Width))
	}
	return m.paint(green, fit(fmt.Sprintf("%d exclusions", len(m.Masks())), m.Width))
}

// Safe escapes untrusted names and diagnostics, including ESC, without adding
// surrounding quotation marks. Control characters cannot alter the terminal.
func Safe(s string) string {
	quoted := strconv.QuoteToGraphic(s)
	return quoted[1 : len(quoted)-1]
}
func size(n uint64) string {
	units := []string{"B", "KiB", "MiB", "GiB", "TiB", "PiB"}
	f := float64(n)
	i := 0
	for f >= 1024 && i < len(units)-1 {
		f /= 1024
		i++
	}
	return fmt.Sprintf("%.1f %s", f, units[i])
}
func estimate(mode config.DeleteMode, ts []cleanup.Target, s scan.Snapshot) string {
	label := "Selected size"
	if mode == config.Permanent {
		label = "Estimated space to reclaim"
	}
	n, state := cleanup.Estimate(ts, s)
	if state == scan.Unknown {
		return label + ": Size unknown · Unknown"
	}
	return fmt.Sprintf("%s: %s · %s", label, size(n), state)
}
func (m *Model) View() string { v, _ := m.render(); return v }

func (m *Model) diagnostics() []string {
	var lines []string
	if m.Message != "" {
		lines = append(lines, Safe(m.Message))
	}
	if e, ok := m.current(); ok && m.Scanned {
		if v := m.Snapshot.Entries[e.Path]; v.Err != nil {
			lines = append(lines, "Item detail: "+Safe(v.Err.Error()))
		}
	}
	return lines
}

func (m *Model) render() (string, int) {
	if m.Screen == "list" || m.Screen == "" || m.Screen == "filter" {
		return m.renderBrowser()
	}
	if m.Screen == "masks" {
		return m.renderMasks()
	}
	if m.Screen == "help" {
		return m.renderHelp()
	}
	lines := []string{"DISKCLEANER  /  manual disk cleanup", Safe(m.Root), ""}
	footer := "Esc back · ? help"
	body := []string{}
	switch m.Screen {
	case "details":
		body = append(body, "ITEM DETAILS", "Current folder: "+Safe(m.Root))
		if entry, ok := m.current(); ok {
			body = append(body, Safe(entry.Path))
		}
		if m.Scanned && !m.LastScanAt.IsZero() {
			body = append(body, m.scanStatus(true))
		}
		body = append(body, m.diagnostics()...)
		footer = "↑/↓ or PgUp/PgDn scroll · Esc back"
	case "scanning":
		lines[1] = tailFit(Safe(m.Root), m.Width)
		body = append(body, "SCANNING", fmt.Sprintf("Processed: %d / %d items", m.Progress.Completed, m.Progress.Total), "Elapsed: "+m.Elapsed.String())
		if m.Progress.Current != "" {
			body = append(body, "Current: "+tailFit(Safe(m.Progress.Current), max(3, m.Width-9)))
		} else {
			body = append(body, "Reading directory / preparing next item...")
		}
		body = append(body, "Item counts do not predict remaining time.")
		footer = "Esc / Ctrl+C cancel"
	case "previewing", "cleaning":
		body = append(body, "Working... "+m.Screen, "Esc cancel — please wait for the final report.")
		footer = "Esc / Ctrl+C cancel"
	case "scan-confirm":
		body = append(body, "Start scan", "Analysis may take a long time. Each immediate item is measured separately.")
		if len(m.Masks()) == 0 {
			body = append(body, m.paint(red, "No exclusions set. Exclusions are optional; Esc then e to edit."))
		}
		body = append(body, "Enter Start scan · Esc Back")
	case "root":
		body = append(body, "Open directory (absolute path, ~, or a path relative to the current root)", "> "+Safe(m.Input), "Enter open · Backspace edit")
		footer = "Esc back"
	case "settings":
		body = append(body, "Deletion mode: "+string(m.Settings.Mode), "←/→ or Space changes mode and saves automatically.")
		if m.Settings.Mode == config.Permanent {
			body = append(body, m.paint(red, "Permanent deletion cannot be undone. Every operation requires confirmation."))
		}
	case "preview":
		body = append(body, "DRY RUN — no files changed", estimate(m.Settings.Mode, m.Preview.Targets, m.Snapshot), "Contents may change before deletion.")
		for _, p := range m.Preview.Paths {
			body = append(body, Safe(p))
		}
		footer = "↑/↓ scroll · Esc back"
	case "confirm":
		color := amber
		if m.ConfirmedMode == config.Permanent {
			color = red
		}
		body = append(body, m.paint(color, "CONFIRM CLEANUP — "+strings.ToUpper(string(m.ConfirmedMode))), estimate(m.ConfirmedMode, m.Confirmed, m.Snapshot))
		if hidden := m.hiddenSelections(); hidden > 0 {
			body = append(body, m.paint(amber, fmt.Sprintf("Hidden selected items: %d (included).", hidden)))
		}
		body = append(body, "New ordinary contents in selected folders are included.", "Checks and deletion are not atomic; concurrent changes remain a risk.")
		for _, t := range m.Confirmed {
			body = append(body, Safe(t.Path))
		}
		footer = "Enter execute ONCE · Esc cancel · ↑/↓ scroll"
	case "results":
		body = append(body, "CLEANUP REPORT — sizes and selection cleared.", "Space reclaimed was not measured. Start a new scan manually.")
		for _, r := range m.Results {
			line := r.Outcome.String() + "  " + Safe(r.Path)
			if r.Err != nil {
				line += " — " + Safe(r.Err.Error())
			}
			color := green
			if r.Outcome == cleanup.Failed {
				color = red
			} else if r.Outcome == cleanup.Skipped {
				color = amber
			}
			body = append(body, m.paint(color, line))
		}
		footer = "↑/↓ scroll · Esc back"

	}
	width := m.Width
	if width < 12 {
		return "Resize", 0
	}
	height := m.Height
	if height < 8 {
		return "Resize", 0
	}
	footerLines := wrap(strings.Split(footer, "\n"), width)
	available := height - len(footerLines) - 1
	if available < 1 {
		return "Resize", 0
	}
	content := wrap(lines[1:], width)
	if m.Message != "" && m.Screen != "details" {
		content = append(content, wrap([]string{Safe(m.Message)}, width)...)
	}
	content = append(content, wrap(body, width)...)
	scrollable := false
	switch m.Screen {
	case "details", "preview", "confirm", "results", "root":
		scrollable = true
	}
	showIndicator := scrollable && len(content) > available
	start, maxStart := 0, 0
	if showIndicator {
		available--
		maxStart = len(content) - available
		start = min(max(0, m.Offset), maxStart)
	}
	end := min(len(content), start+available)
	result := []string{m.paint(cyan, "DISKCLEANER")}
	result = append(result, content[start:end]...)
	if showIndicator {
		indicator := fmt.Sprintf("Lines %d–%d / %d", start+1, end, len(content))
		if len([]rune(indicator)) > width {
			indicator = "More: ↑/↓"
		}
		result = append(result, indicator)
	}
	result = append(result, footerLines...)
	return strings.Join(result, "\n"), maxStart
}

// Conservative cell accounting keeps wide Unicode names inside the viewport.
// Names remain exact in stored paths; wrapping affects display only.
func wrap(lines []string, width int) []string {
	out := []string{}
	for _, line := range lines {
		style, endStyle := "", ""
		if strings.HasPrefix(line, "\x1b[") && strings.HasSuffix(line, reset) {
			prefix, rest, found := strings.Cut(line, "m")
			if found {
				style, endStyle = prefix+"m", reset
				line = strings.TrimSuffix(rest, reset)
			}
		}
		var b strings.Builder
		cells := 0
		for _, r := range line {
			n := 1
			if r > 127 {
				n = 2
			}
			if cells+n > width {
				out = append(out, style+b.String()+endStyle)
				b.Reset()
				cells = 0
			}
			b.WriteRune(r)
			cells += n
		}
		out = append(out, style+b.String()+endStyle)
	}
	return out
}
