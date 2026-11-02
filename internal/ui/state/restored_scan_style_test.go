package state

import (
	"diskcleaner/internal/cleanup"
	"diskcleaner/internal/scan"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestRestoredScanWarningIsRedWithOriginalTimeWithoutDuplicateSummary(t *testing.T) {
	m := fixture(t)
	scanNow(t, m)
	fresh := New(m.Root, m.Settings, m.Guard, m.ConfigPath)
	fresh.Width, fresh.Height = 180, 24
	fresh.NoColor = false
	stamp := m.LastScanAt.Local().Format("2006-01-02 15:04:05 MST")
	view := fresh.View()
	if !strings.Contains(view, "\x1b[31mSaved sizes may have changed") || !strings.Contains(plainView(view), "last scan "+m.LastScanAt.Local().Format("2006-01-02 15:04")) {
		t.Fatal("saved-size warning is not red or lacks the original successful scan time", view)
	}
	if strings.Contains(plainView(view), "Last scan "+m.LastScanAt.Local().Format("15:04")+" | Complete") {
		t.Fatal("restored browser duplicated the scan summary above the warning", view)
	}
	fresh.Key("v")
	details := fresh.View()
	if !strings.Contains(details, "\x1b[31mSaved sizes may have changed") || !strings.Contains(plainView(details), "Last scan: "+stamp) || strings.Contains(details, "\x1b[31mLast scan:") {
		t.Fatal("details lost red warning or normally styled full last-scan timestamp", details)
	}
	if !strings.Contains(plainView(details), "last scan "+m.LastScanAt.Local().Format("2006-01-02 15:04")+"; press s to rescan.") {
		t.Fatal("details warning lacks full successful scan date and time", details)
	}
}

func TestRestoredScanColorsRespectNoColorWithoutChangingLayout(t *testing.T) {
	m := fixture(t)
	scanNow(t, m)
	fresh := New(m.Root, m.Settings, m.Guard, m.ConfigPath)
	for _, screen := range []string{"list", "details"} {
		for _, width := range []int{32, 64, 80, 180} {
			fresh.Screen, fresh.Width, fresh.Height = screen, width, 24
			fresh.NoColor = false
			colored := fresh.View()
			// The terminal adapter maps NO_COLOR to this state flag.
			fresh.NoColor = true
			plain := fresh.View()
			if strings.Contains(plain, "\x1b[") || plain != plainView(colored) {
				t.Fatal("NO_COLOR rendering changed warning layout or retained ANSI", screen, width, plain)
			}
		}
	}
}

func TestRestoredScanWarningRetainsTimeInNarrowBrowser(t *testing.T) {
	m := fixture(t)
	scanNow(t, m)
	fresh := New(m.Root, m.Settings, m.Guard, m.ConfigPath)
	for _, width := range []int{32, 40, 63, 64, 80} {
		fresh.Width, fresh.Height, fresh.NoColor = width, 12, false
		view := fresh.View()
		plain := plainView(view)
		warning := ""
		for _, line := range strings.Split(plain, "\n") {
			if strings.Contains(line, "Saved") {
				warning = line
				break
			}
		}
		if !strings.Contains(warning, "may have changed") || !strings.Contains(warning, m.LastScanAt.Local().Format("15:04")) {
			t.Fatal("narrow warning dropped uncertainty or successful scan time", width, plain)
		}
		for _, line := range strings.Split(plain, "\n") {
			if !strings.Contains(line, "Saved") && (strings.Contains(line, "Last scan ") || strings.Contains(line, "Scan "+m.LastScanAt.Local().Format("15:04"))) {
				t.Fatal("restored browser retained a separate scan summary", width, line)
			}
		}
		if !strings.Contains(view, "\x1b[31mSaved") {
			t.Fatal("narrow saved-size warning lost red styling", width, view)
		}
		if len(strings.Split(view, "\n")) != fresh.Height {
			t.Fatal("saved-size warning changed browser frame height", width, view)
		}
		for _, line := range strings.Split(view, "\n") {
			if cells(line) > width {
				t.Fatal("saved-size warning overflowed terminal width", width, line)
			}
		}
	}
}

func TestSuccessfulRescanClearsRestoredRedStatus(t *testing.T) {
	m := fixture(t)
	scanNow(t, m)
	fresh := New(m.Root, m.Settings, m.Guard, m.ConfigPath)
	fresh.Width, fresh.Height, fresh.NoColor = 180, 24, false
	scanNow(t, fresh)
	for _, screen := range []string{"list", "details"} {
		fresh.Screen = screen
		view := fresh.View()
		if strings.Contains(view, "\x1b[31mLast scan") || strings.Contains(plainView(view), "Saved sizes may have changed") {
			t.Fatal("successful rescan retained restored warning or red last-scan status", screen, view)
		}
		if !strings.Contains(plainView(view), "Last scan") {
			t.Fatal("successful rescan removed last-scan information", screen, view)
		}
	}
}

func TestRestoredCompleteStatusAloneIsRedAndPreservesRowStyles(t *testing.T) {
	m := fixture(t)
	m.Height, m.NoColor = 24, false
	m.Scanned = true
	m.Settings.Masks = []string{"excluded"}
	m.Entries = nil
	m.Snapshot = scan.Snapshot{Entries: map[string]scan.Measurement{}}
	for _, name := range []string{"complete", "partial", "unknown", "excluded"} {
		path := filepath.Join(m.Root, name)
		m.Entries = append(m.Entries, scan.Entry{Name: name, Path: path})
		state := scan.Complete
		if name == "partial" {
			state = scan.Partial
		} else if name == "unknown" {
			state = scan.Unknown
		}
		m.Snapshot.Entries[path] = scan.Measurement{State: state, Bytes: 4096}
	}
	redComplete := regexp.MustCompile("\\x1b\\[31mComplete\\x1b\\[(32|37|39)m")
	row := func(view, name string) string {
		lines := strings.Split(view, "\n")
		for i, line := range lines {
			if strings.Contains(plainView(line), "[ ] "+name) || strings.Contains(plainView(line), "[x] "+name) {
				if m.Width < 64 && i+1 < len(lines) {
					return line + "\n" + lines[i+1]
				}
				return line
			}
		}
		t.Fatal("test entry row missing", name, view)
		return ""
	}
	for _, width := range []int{32, 140} {
		for _, cursor := range []int{0, 1} {
			for _, selected := range []bool{false, true} {
				m.Width, m.Cursor = width, cursor
				path := m.Entries[0].Path
				delete(m.Selected, path)
				if selected {
					m.Selected[path] = cleanup.Target{Path: path}
				}
				m.RestoredScan = false
				fresh := m.View()
				m.RestoredScan = true
				restored := m.View()
				for _, name := range []string{"complete", "partial", "unknown", "excluded"} {
					before, after := row(fresh, name), row(restored, name)
					if name == "complete" {
						if !redComplete.MatchString(after) || redComplete.MatchString(before) {
							t.Fatal("only restored Complete status should be red", width, cursor, selected, after)
						}
						after = redComplete.ReplaceAllString(after, "Complete")
					}
					if before != after {
						t.Fatal("restored status changed ordinary row styling, selection or background", width, cursor, selected, name, before, after)
					}
				}
			}
		}
	}
}
