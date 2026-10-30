package state

import (
	"diskcleaner/internal/scan"
	"sync"
	"time"
)

// The worker publishes only to its own mailbox; all Model changes stay on the
// UI thread. A completed or cancelled scan cannot update a later scan's state.
type progressMailbox struct {
	mu    sync.Mutex
	value scan.Progress
}

func (p *progressMailbox) publish(value scan.Progress) {
	p.mu.Lock()
	p.value = value
	p.mu.Unlock()
}

func (m *Model) RefreshProgress(now time.Time) {
	if !m.Busy || m.Screen != "scanning" || m.progress == nil {
		return
	}
	m.progress.mu.Lock()
	m.Progress = m.progress.value
	m.progress.mu.Unlock()
	m.Elapsed = max(0, now.Sub(m.startedAt)).Truncate(time.Second)
}
