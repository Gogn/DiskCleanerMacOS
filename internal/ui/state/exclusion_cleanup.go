package state

import (
	"context"
	"diskcleaner/internal/cleanup"
	"diskcleaner/internal/policy"
	"errors"
	"path/filepath"
	"strings"
)

func descendantOf(path, directory string) bool {
	relative, err := filepath.Rel(directory, path)
	return err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func (m *Model) applyCleanupResult(e Event, cancelled bool) {
	defer m.releaseCleanupCacheLock()
	attempted := m.cleanupAttempted
	if e.Kind == "cleanup" {
		m.Results = e.Results
		m.exclusionQueue = nil
		attempted = m.Confirmed
		if !cancelled {
			for _, result := range e.Results {
				var excluded *policy.ExcludedNameError
				if result.Outcome != cleanup.Skipped || !errors.As(result.Err, &excluded) {
					continue
				}
				for _, target := range m.Confirmed {
					if target.Path == result.Path && target.IsDir && descendantOf(excluded.Path, target.Path) {
						m.exclusionQueue = append(m.exclusionQueue, target)
						break
					}
				}
			}
		}
	} else {
		for _, result := range e.Results {
			for i := range m.Results {
				if m.Results[i].Path == result.Path {
					m.Results[i] = result
					break
				}
			}
		}
	}
	if cancelled {
		m.exclusionQueue = nil
	}
	err := m.refreshAfterCleanup(attempted, e.Results)
	m.cleanupAttempted = nil
	m.Cursor = 0
	m.normalizeCursor()
	m.nextExclusionConfirmation()
	if err != nil {
		m.Message = "Could not refresh after cleanup: " + err.Error()
	}
}

func (m *Model) nextExclusionConfirmation() {
	m.exclusionDelete = false
	m.Offset = 0
	m.Screen = "results"
	if len(m.exclusionQueue) > 0 {
		m.Screen = "exclusion-confirm"
	}
}

func (m *Model) exclusionConfirmationKey(key string) Job {
	if len(m.exclusionQueue) == 0 {
		m.nextExclusionConfirmation()
		return nil
	}
	switch key {
	case "esc":
		m.exclusionQueue = nil
		m.nextExclusionConfirmation()
	case "left":
		m.exclusionDelete = false
	case "right":
		m.exclusionDelete = true
	case "tab":
		m.exclusionDelete = !m.exclusionDelete
	case "enter":
		target := m.exclusionQueue[0]
		m.exclusionQueue = m.exclusionQueue[1:]
		if !m.exclusionDelete {
			m.nextExclusionConfirmation()
			return nil
		}
		m.exclusionDelete = false
		targets := []cleanup.Target{target}
		mode, guard, trash, remove := m.ConfirmedMode, m.Guard, m.Trash, m.Remove
		m.cleanupAttempted = targets
		if !m.beginCleanupCache(targets) {
			m.exclusionQueue = append([]cleanup.Target{target}, m.exclusionQueue...)
			m.Offset = 0
			m.Message = "Could not invalidate saved sizes; cleanup was not started. Retry when cache storage is available."
			return nil
		}
		return m.start("cleaning", func(ctx context.Context) Event {
			results := cleanup.ExecuteWithExclusionOverrides(ctx, mode, targets, guard, trash, remove, map[cleanup.Target]bool{target: true})
			return Event{Kind: "cleanup-override", Results: results}
		})
	default:
		m.scroll(key)
	}
	return nil
}

func (m *Model) currentExcludedPath() string {
	if len(m.exclusionQueue) == 0 {
		return ""
	}
	for _, result := range m.Results {
		var excluded *policy.ExcludedNameError
		if result.Path == m.exclusionQueue[0].Path && errors.As(result.Err, &excluded) {
			return excluded.Path
		}
	}
	return ""
}
