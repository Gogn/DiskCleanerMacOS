package state

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// A cleanup owns an exclusive OS lock until its final refresh. Cache readers
// and writers take shared locks, so no other session can publish a snapshot of
// a tree while the cleanup runner may still change it. Process exit releases
// locks automatically; the private lock file contains no cache data.
func (m *Model) lockScanCache(exclusive bool) (*os.File, error) {
	if m.scanCachePath() == "" {
		return nil, nil
	}
	path := m.scanCachePath() + ".lock"
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0600)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		file.Close()
		return nil, errors.New("invalid scan cache lock file")
	}
	if err = file.Chmod(0600); err != nil {
		file.Close()
		return nil, err
	}
	operation := syscall.LOCK_SH | syscall.LOCK_NB
	if exclusive {
		operation = syscall.LOCK_EX | syscall.LOCK_NB
	}
	deadline := time.Now().Add(250 * time.Millisecond)
	for {
		err = syscall.Flock(int(file.Fd()), operation)
		if err == nil {
			return file, nil
		}
		if err != syscall.EWOULDBLOCK && err != syscall.EAGAIN {
			file.Close()
			return nil, err
		}
		if !time.Now().Before(deadline) {
			file.Close()
			return nil, errors.New("scan cache is in use by cleanup")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
func unlockScanCache(file *os.File) {
	if file != nil {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
	}
}
func (m *Model) releaseCleanupCacheLock() {
	unlockScanCache(m.cleanupCacheLock)
	m.cleanupCacheLock = nil
}
