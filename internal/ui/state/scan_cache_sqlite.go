package state

import (
	"context"
	"database/sql"
	"diskcleaner/internal/scan"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	_ "modernc.org/sqlite"
)

var errCacheConflict = errors.New("scan cache changed in another session")

func (m *Model) scanCachePath() string {
	if m.ConfigPath == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(m.ConfigPath), "scan-cache.sqlite")
}
func maskKey(masks []string) string {
	data, _ := json.Marshal(normalizedMasks(masks))
	return string(data)
}
func snapshotKey(cached cachedScan) string {
	data, _ := json.Marshal(encodeDiskScan(cached))
	return string(data)
}

// Every operation closes its own connection. DELETE journals inherit the
// precreated 0600 database's permissions; no persistent WAL sidecars remain.
func (m *Model) cacheTransaction(work func(context.Context, *sql.Tx) error) error {
	path := m.scanCachePath()
	if path == "" {
		return nil
	}
	if m.cleanupCacheLock == nil {
		lock, err := m.lockScanCache(false)
		if err != nil {
			return err
		}
		defer unlockScanCache(lock)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err == nil {
		err = f.Close()
	} else if os.IsExist(err) {
		err = nil
	}
	if err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > 128<<20 {
		return errors.New("invalid scan cache file")
	}
	if err = os.Chmod(path, 0600); err != nil {
		return err
	}
	for _, suffix := range []string{"-journal", "-wal", "-shm"} {
		if side, err := os.Lstat(path + suffix); err == nil && !side.Mode().IsRegular() {
			return errors.New("invalid scan cache sidecar")
		}
	}
	uri := url.URL{Scheme: "file", Path: path}
	query := uri.Query()
	query.Add("_pragma", "busy_timeout(250)")
	query.Add("_pragma", "foreign_keys(1)")
	query.Set("_txlock", "immediate")
	uri.RawQuery = query.Encode()
	db, err := sql.Open("sqlite", uri.String())
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var version int
	if err = db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version != 0 && version != 1 {
		return errors.New("unsupported scan cache version")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if version == 0 {
		for _, statement := range []string{
			`CREATE TABLE IF NOT EXISTS meta (id INTEGER PRIMARY KEY CHECK(id=1), epoch INTEGER NOT NULL)`,
			`CREATE TABLE IF NOT EXISTS roots (path TEXT PRIMARY KEY, revision INTEGER NOT NULL, masks TEXT NOT NULL, identity BLOB, scanned_at TEXT, state INTEGER)`,
			`CREATE TABLE IF NOT EXISTS entries (root TEXT NOT NULL REFERENCES roots(path), path TEXT NOT NULL, position INTEGER NOT NULL, name TEXT NOT NULL, is_dir INTEGER NOT NULL, is_symlink INTEGER NOT NULL, identity BLOB, bytes TEXT NOT NULL, state INTEGER NOT NULL, had_error INTEGER NOT NULL, PRIMARY KEY(root,path))`,
			`INSERT OR IGNORE INTO meta(id,epoch) VALUES(1,0)`,
			`PRAGMA user_version=1`,
		} {
			if _, err = tx.ExecContext(ctx, statement); err != nil {
				return err
			}
		}
	}
	var epoch int64
	if err = tx.QueryRowContext(ctx, "SELECT epoch FROM meta WHERE id=1").Scan(&epoch); err != nil {
		return err
	}
	if epoch < 0 {
		return errors.New("invalid cache epoch")
	}
	if err = work(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}

func writeRoot(ctx context.Context, tx *sql.Tx, path string, cached cachedScan, masks string, revision int64) error {
	identity, _ := json.Marshal(cached.root)
	if _, err := tx.ExecContext(ctx, `INSERT INTO roots(path,revision,masks,identity,scanned_at,state) VALUES(?,?,?,?,?,?) ON CONFLICT(path) DO UPDATE SET revision=excluded.revision,masks=excluded.masks,identity=excluded.identity,scanned_at=excluded.scanned_at,state=excluded.state`, path, revision+1, masks, identity, cached.scannedAt.Format(time.RFC3339Nano), cached.snapshot.State); err != nil {
		return err
	}
	// Upsert individual entries, preserving untouched rows byte-for-byte.
	keep := map[string]bool{}
	for position, entry := range cached.snapshot.Listing {
		value, ok := cached.snapshot.Entries[entry.Path]
		if !ok {
			value.State = scan.Unknown
		}
		identity, _ := json.Marshal(cached.entries[entry.Path])
		keep[entry.Path] = true
		_, err := tx.ExecContext(ctx, `INSERT INTO entries(root,path,position,name,is_dir,is_symlink,identity,bytes,state,had_error) VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT(root,path) DO UPDATE SET position=excluded.position,name=excluded.name,is_dir=excluded.is_dir,is_symlink=excluded.is_symlink,identity=excluded.identity,bytes=excluded.bytes,state=excluded.state,had_error=excluded.had_error WHERE position!=excluded.position OR name!=excluded.name OR is_dir!=excluded.is_dir OR is_symlink!=excluded.is_symlink OR identity IS NOT excluded.identity OR bytes!=excluded.bytes OR state!=excluded.state OR had_error!=excluded.had_error`, path, entry.Path, position, entry.Name, entry.IsDir, entry.IsSymlink, identity, strconv.FormatUint(value.Bytes, 10), value.State, value.Err != nil)
		if err != nil {
			return err
		}
	}
	rows, err := tx.QueryContext(ctx, "SELECT path FROM entries WHERE root=?", path)
	if err != nil {
		return err
	}
	var removed []string
	for rows.Next() {
		var entry string
		if err = rows.Scan(&entry); err != nil {
			rows.Close()
			return err
		}
		if !keep[entry] {
			removed = append(removed, entry)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, entry := range removed {
		if _, err = tx.ExecContext(ctx, "DELETE FROM entries WHERE root=? AND path=?", path, entry); err != nil {
			return err
		}
	}
	return nil
}
func tombstoneRoot(ctx context.Context, tx *sql.Tx, path string) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM entries WHERE root=?", path); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, "UPDATE roots SET revision=revision+1,identity=NULL,scanned_at=NULL,state=NULL WHERE path=?", path)
	return err
}
func readRoot(ctx context.Context, tx *sql.Tx, path, masks string) (cachedScan, int64, bool, error) {
	var revision int64
	var storedMasks string
	var identity []byte
	var scanned sql.NullString
	var state sql.NullInt64
	err := tx.QueryRowContext(ctx, "SELECT revision,masks,identity,scanned_at,state FROM roots WHERE path=?", path).Scan(&revision, &storedMasks, &identity, &scanned, &state)
	if errors.Is(err, sql.ErrNoRows) {
		return cachedScan{}, 0, false, nil
	}
	if err != nil {
		return cachedScan{}, 0, false, err
	}
	if identity == nil || storedMasks != masks {
		return cachedScan{}, revision, false, nil
	}
	if !state.Valid || state.Int64 < 0 || state.Int64 > int64(scan.Skipped) {
		return cachedScan{}, revision, false, errors.New("invalid root state")
	}
	saved := diskScan{Identities: map[string]fileIdentity{}, Measurements: map[string]diskMeasurement{}, State: scan.State(state.Int64)}
	if len(identity) > 4096 || json.Unmarshal(identity, &saved.Root) != nil {
		return cachedScan{}, revision, false, errors.New("invalid root identity")
	}
	saved.ScannedAt, err = time.Parse(time.RFC3339Nano, scanned.String)
	if err != nil {
		return cachedScan{}, revision, false, err
	}
	rows, err := tx.QueryContext(ctx, "SELECT path,name,is_dir,is_symlink,identity,bytes,state,had_error FROM entries WHERE root=? ORDER BY position", path)
	if err != nil {
		return cachedScan{}, revision, false, err
	}
	defer rows.Close()
	size := 0
	for rows.Next() {
		var entry scan.Entry
		var identity []byte
		var amount string
		var value diskMeasurement
		var rawState int
		if err = rows.Scan(&entry.Path, &entry.Name, &entry.IsDir, &entry.IsSymlink, &identity, &amount, &rawState, &value.HadError); err != nil {
			return cachedScan{}, revision, false, err
		}
		size += len(entry.Path) + len(entry.Name) + len(identity) + len(amount) + 64
		if size > maxScanCacheBytes {
			return cachedScan{}, revision, false, errors.New("scan cache root exceeds limit")
		}
		if rawState < 0 || rawState > int(scan.Skipped) {
			return cachedScan{}, revision, false, errors.New("invalid measurement state")
		}
		value.State = scan.State(rawState)
		value.Bytes, err = strconv.ParseUint(amount, 10, 64)
		if err != nil {
			return cachedScan{}, revision, false, err
		}
		var info fileIdentity
		if identity != nil && json.Unmarshal(identity, &info) != nil {
			return cachedScan{}, revision, false, errors.New("invalid entry identity")
		}
		saved.Listing = append(saved.Listing, entry)
		saved.Identities[entry.Path] = info
		saved.Measurements[entry.Path] = value
	}
	if err = rows.Err(); err != nil {
		return cachedScan{}, revision, false, err
	}
	cached, valid := decodeDiskScan(path, saved)
	return cached, revision, valid, nil
}
func (m *Model) initCacheMaps() {
	if m.cacheBaseline == nil {
		m.cacheBaseline = map[string]string{}
	}
	if m.cacheRevisions == nil {
		m.cacheRevisions = map[string]int64{}
	}
	if m.scanCache == nil {
		m.scanCache = map[string]cachedScan{}
	}
}
func (m *Model) loadScanCache() {
	if m.scanCachePath() == "" {
		return
	}
	m.initCacheMaps()
	err := m.cacheTransaction(func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT epoch FROM meta WHERE id=1").Scan(&m.cacheEpoch)
	})
	m.cacheInitialized = err == nil
}
func (m *Model) refreshRootCache(root string) bool {
	if m.scanCachePath() == "" {
		return true
	}
	m.initCacheMaps()
	old, hadOld := m.scanCache[root]
	previousEpoch, previousRevision := m.cacheEpoch, m.cacheRevisions[root]
	previousBaseline := m.cacheBaseline[root]
	var epoch, revision int64
	var cached cachedScan
	var found bool
	err := m.cacheTransaction(func(ctx context.Context, tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, "SELECT epoch FROM meta WHERE id=1").Scan(&epoch); err != nil {
			return err
		}
		var err error
		cached, revision, found, err = readRoot(ctx, tx, root, maskKey(m.Masks()))
		return err
	})
	if err != nil {
		// A failed read is not an intentional eviction. Retain the baseline
		// so a later write cannot turn this temporary miss into a tombstone.
		m.cacheWriteFailed = true
		return false
	}
	if epoch != m.cacheEpoch {
		m.scanCache = map[string]cachedScan{}
		m.cacheBaseline = map[string]string{}
		m.cacheRevisions = map[string]int64{}
	}
	m.cacheInitialized = true
	m.cacheEpoch = epoch
	m.cacheRevisions[root] = revision
	if hadOld && !old.volatile && epoch == previousEpoch && revision == previousRevision && snapshotKey(old) != previousBaseline {
		// A successful local scan may not have reached disk during a lock or
		// I/O failure. Keep it only after verifying no external invalidation.
		return true
	}
	if found {
		if hadOld && old.scannedAt.Equal(cached.scannedAt) && old.root.sameObject(cached.root) {
			cached.restored = old.restored
		}
		m.scanCache[root] = cached
		m.cacheBaseline[root] = snapshotKey(cached)
	} else {
		delete(m.scanCache, root)
		delete(m.cacheBaseline, root)
	}
	return true
}
func (m *Model) persistScanCache() {
	if m.scanCachePath() == "" {
		return
	}
	m.initCacheMaps()
	changes := map[string]*cachedScan{}
	for root, cached := range m.scanCache {
		if cached.volatile {
			continue
		}
		if snapshotKey(cached) != m.cacheBaseline[root] {
			copy := cached
			changes[root] = &copy
		}
	}
	for root := range m.cacheBaseline {
		if _, ok := m.scanCache[root]; !ok {
			changes[root] = nil
		}
	}
	if len(changes) == 0 {
		return
	}
	err := m.cacheTransaction(func(ctx context.Context, tx *sql.Tx) error {
		var epoch int64
		if err := tx.QueryRowContext(ctx, "SELECT epoch FROM meta WHERE id=1").Scan(&epoch); err != nil {
			return err
		}
		if !m.cacheInitialized || epoch != m.cacheEpoch {
			return errCacheConflict
		}
		for root, cached := range changes {
			var revision int64
			err := tx.QueryRowContext(ctx, "SELECT revision FROM roots WHERE path=?", root).Scan(&revision)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			if revision != m.cacheRevisions[root] {
				return errCacheConflict
			}
			if cached == nil {
				if err = tombstoneRoot(ctx, tx, root); err != nil {
					return err
				}
			} else {
				if err = writeRoot(ctx, tx, root, *cached, maskKey(m.Masks()), revision); err != nil {
					return err
				}
			}
		}
		return nil
	})
	m.cacheWriteFailed = err != nil
	if err != nil {
		if errors.Is(err, errCacheConflict) {
			m.scanCache = nil
			m.cacheBaseline = nil
			m.cacheRevisions = nil
			m.resetScan()
			m.refreshRootCache(m.Root)
		}
		return
	}
	for root, cached := range changes {
		m.cacheRevisions[root]++
		if cached == nil {
			delete(m.cacheBaseline, root)
		} else {
			m.cacheBaseline[root] = snapshotKey(*cached)
		}
	}
}

// clearSavedScans keeps root tombstones. An old process
// cannot repopulate a cleared database using its stale epoch or revision.
func (m *Model) clearSavedScans() {
	if m.scanCachePath() == "" {
		return
	}
	err := m.cacheTransaction(func(ctx context.Context, tx *sql.Tx) error {
		for _, query := range []string{"DELETE FROM entries", "UPDATE roots SET revision=revision+1,identity=NULL,scanned_at=NULL,state=NULL", "UPDATE meta SET epoch=epoch+1 WHERE id=1"} {
			if _, err := tx.ExecContext(ctx, query); err != nil {
				return err
			}
		}
		return tx.QueryRowContext(ctx, "SELECT epoch FROM meta WHERE id=1").Scan(&m.cacheEpoch)
	})
	m.cacheWriteFailed = err != nil
	m.cacheInitialized = err == nil
	m.cacheBaseline = nil
	m.cacheRevisions = nil
}

func (m *Model) invalidateSavedPaths(paths []string) error {
	if m.scanCachePath() == "" || len(paths) == 0 {
		return nil
	}
	var epoch int64
	err := m.cacheTransaction(func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, "SELECT path FROM roots WHERE identity IS NOT NULL")
		if err != nil {
			return err
		}
		var roots []string
		for rows.Next() {
			var root string
			if err = rows.Scan(&root); err != nil {
				rows.Close()
				return err
			}
			roots = append(roots, root)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, root := range roots {
			evict := false
			for _, path := range paths {
				if pathWithin(root, path) {
					evict = true
					break
				}
			}
			if evict {
				if err = tombstoneRoot(ctx, tx, root); err != nil {
					return err
				}
				continue
			}
			rows, err = tx.QueryContext(ctx, "SELECT path FROM entries WHERE root=?", root)
			if err != nil {
				return err
			}
			var affected []string
			for rows.Next() {
				var entry string
				if err = rows.Scan(&entry); err != nil {
					rows.Close()
					return err
				}
				for _, path := range paths {
					if pathWithin(entry, path) || pathWithin(path, entry) {
						affected = append(affected, entry)
						break
					}
				}
			}
			err = rows.Err()
			rows.Close()
			if err != nil {
				return err
			}
			for _, entry := range affected {
				if _, err = tx.ExecContext(ctx, "UPDATE entries SET identity=NULL,bytes='0',state=?,had_error=0 WHERE root=? AND path=?", scan.Unknown, root, entry); err != nil {
					return err
				}
			}
			if len(affected) > 0 {
				if _, err = tx.ExecContext(ctx, "UPDATE roots SET revision=revision+1,state=? WHERE path=?", scan.Partial, root); err != nil {
					return err
				}
			}
		}
		if _, err = tx.ExecContext(ctx, "UPDATE meta SET epoch=epoch+1 WHERE id=1"); err != nil {
			return err
		}
		return tx.QueryRowContext(ctx, "SELECT epoch FROM meta WHERE id=1").Scan(&epoch)
	})
	if err != nil {
		return fmt.Errorf("invalidate saved sizes: %w", err)
	}
	m.cacheEpoch = epoch
	m.cacheInitialized = true
	// Refresh only already visited roots; other database roots stay lazy.
	roots := make([]string, 0, len(m.scanCache))
	for root := range m.scanCache {
		roots = append(roots, root)
	}
	m.cacheBaseline = nil
	m.cacheRevisions = nil
	for _, root := range roots {
		m.refreshRootCache(root)
	}
	return nil
}

// A worker keeps the revision from scan start, even if other model reads later
// observe a newer database epoch. Delivering an old event cannot rebase it.
type scanCacheToken struct {
	verified        bool
	root            string
	epoch, revision int64
}

func (m *Model) validateScanCacheToken(token *scanCacheToken) error {
	if token == nil || m.scanCachePath() == "" {
		return nil
	}
	if token.root != m.Root {
		return errCacheConflict
	}
	if !token.verified {
		m.cacheWriteFailed = true
		return nil
	}
	err := m.cacheTransaction(func(ctx context.Context, tx *sql.Tx) error {
		var epoch, revision int64
		if err := tx.QueryRowContext(ctx, "SELECT epoch FROM meta WHERE id=1").Scan(&epoch); err != nil {
			return err
		}
		err := tx.QueryRowContext(ctx, "SELECT revision FROM roots WHERE path=?", token.root).Scan(&revision)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if epoch != token.epoch || revision != token.revision {
			return errCacheConflict
		}
		return nil
	})
	if errors.Is(err, errCacheConflict) {
		return err
	}
	// Storage failure remains a nonfatal cache failure for explicit measurement.
	if err != nil {
		m.cacheWriteFailed = true
	}
	return nil
}
