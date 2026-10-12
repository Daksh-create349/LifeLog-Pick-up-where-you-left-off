package collector

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lifelog/lifelog/internal/events"
	"github.com/lifelog/lifelog/internal/paths"
)

var (
	activeWatchersMu sync.Mutex
	activeWatchers   = make(map[string]bool)
	eventSeqCounter  uint64
)

// PollerConfig defines the parameters for the filesystem polling collector.
type PollerConfig struct {
	Root           string
	Interval       time.Duration
	DataDir        string
	RecordBaseline bool
	OnEvents       func(batch []events.Event) error
}

type fileMeta struct {
	size    int64
	modTime time.Time
	isDir   bool
}

// FSPoller is a pure-Go periodic snapshot-and-diff filesystem watcher.
type FSPoller struct {
	root           string
	interval       time.Duration
	dataDir        string
	canonDataDir   string
	recordBaseline bool
	onEvents       func(batch []events.Event) error
	snapshot       map[string]fileMeta
}

// NewFSPoller initializes and validates a new filesystem polling collector.
func NewFSPoller(cfg PollerConfig) (*FSPoller, error) {
	if cfg.Root == "" {
		return nil, fmt.Errorf("%w: path cannot be empty", ErrInvalidRoot)
	}

	absRoot, err := filepath.Abs(cfg.Root)
	if err != nil {
		return nil, fmt.Errorf("%w: failed resolving absolute path: %v", ErrInvalidRoot, err)
	}
	absRoot = filepath.Clean(absRoot)

	if err := validateMonitoredRoot(absRoot, cfg.DataDir); err != nil {
		return nil, err
	}

	interval := cfg.Interval
	if interval <= 0 {
		interval = 1 * time.Second
	}

	var canonData string
	if cfg.DataDir != "" {
		if c, err := paths.Canonical(cfg.DataDir); err == nil {
			canonData = c
		} else {
			canonData = filepath.Clean(cfg.DataDir)
		}
	}

	return &FSPoller{
		root:           absRoot,
		interval:       interval,
		dataDir:        filepath.Clean(cfg.DataDir),
		canonDataDir:   canonData,
		recordBaseline: cfg.RecordBaseline,
		onEvents:       cfg.OnEvents,
		snapshot:       make(map[string]fileMeta),
	}, nil
}

func validateMonitoredRoot(root, dataDir string) error {
	// 1. Root cannot be filesystem root
	if root == "/" || root == filepath.VolumeName(root)+"\\" {
		return fmt.Errorf("%w: cannot monitor filesystem root %q", ErrRestrictedPath, root)
	}

	// 2. Root cannot be user's home directory
	home, err := os.UserHomeDir()
	if err == nil && filepath.Clean(home) == root {
		return fmt.Errorf("%w: cannot monitor user home directory directly; please specify a project subdirectory", ErrRestrictedPath)
	}

	// 3. Root cannot be LIFELOG's internal data directory or inside it
	if dataDir != "" {
		cleanData := filepath.Clean(dataDir)
		if root == cleanData || strings.HasPrefix(root, cleanData+string(filepath.Separator)) {
			return fmt.Errorf("%w: cannot monitor LIFELOG internal data directory %q", ErrRestrictedPath, root)
		}
	}

	// 4. Verify path exists and is a directory
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: directory does not exist: %s", ErrInvalidRoot, root)
		}
		return fmt.Errorf("%w: inaccessible: %v", ErrInvalidRoot, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%w: path is not a directory: %s", ErrInvalidRoot, root)
	}

	return nil
}

// Strategy returns the collector implementation name.
func (p *FSPoller) Strategy() string {
	return "fs_poll (pure-Go recursive scanner)"
}

// MonitoredRoot returns the absolute directory path being watched.
func (p *FSPoller) MonitoredRoot() string {
	return p.root
}

func nextEventID() string {
	seq := atomic.AddUint64(&eventSeqCounter, 1)
	return fmt.Sprintf("evt_%d_%d", time.Now().UTC().UnixNano(), seq)
}

// Start begins periodic scanning until the context is cancelled.
func (p *FSPoller) Start(ctx context.Context) error {
	activeWatchersMu.Lock()
	if activeWatchers[p.root] {
		activeWatchersMu.Unlock()
		return fmt.Errorf("%w: %s", ErrAlreadyWatching, p.root)
	}
	activeWatchers[p.root] = true
	activeWatchersMu.Unlock()

	defer func() {
		activeWatchersMu.Lock()
		delete(activeWatchers, p.root)
		activeWatchersMu.Unlock()
	}()

	// Initial scan
	initialSnap, err := p.scan()
	if err != nil {
		return fmt.Errorf("initial scan failed: %w", err)
	}

	if p.recordBaseline && len(initialSnap) > 0 {
		var baselineEvents []events.Event
		now := time.Now().UTC()
		for path, meta := range initialSnap {
			baselineEvents = append(baselineEvents, events.Event{
				ID:        nextEventID(),
				Type:      events.EventCreated,
				Path:      path,
				Size:      meta.size,
				ModTime:   meta.modTime,
				IsDir:     meta.isDir,
				Source:    "fs_poll",
				CreatedAt: now,
			})
		}
		if p.onEvents != nil && len(baselineEvents) > 0 {
			if err := p.onEvents(baselineEvents); err != nil {
				return fmt.Errorf("emitting initial baseline events: %w", err)
			}
		}
	}
	p.snapshot = initialSnap

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			// Check if root still exists
			if _, err := os.Stat(p.root); err != nil {
				return fmt.Errorf("%w: %s (%v)", ErrRootUnavailable, p.root, err)
			}

			currentSnap, err := p.scan()
			if err != nil {
				// Log or return error if scan fails
				return fmt.Errorf("scanning directory %s: %w", p.root, err)
			}

			diffEvents := p.diff(p.snapshot, currentSnap)
			p.snapshot = currentSnap

			if len(diffEvents) > 0 && p.onEvents != nil {
				if err := p.onEvents(diffEvents); err != nil {
					return fmt.Errorf("dispatching filesystem events: %w", err)
				}
			}
		}
	}
}

func (p *FSPoller) scan() (map[string]fileMeta, error) {
	snap := make(map[string]fileMeta)
	var inaccessibleDirs []string

	err := filepath.WalkDir(p.root, func(path string, d fs.DirEntry, err error) error {
		cleanPath := filepath.Clean(path)

		if err != nil {
			// Permission or access error on subdirectory - record inaccessible directory and skip
			inaccessibleDirs = append(inaccessibleDirs, cleanPath)
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip monitored root directory itself
		if cleanPath == p.root {
			return nil
		}

		// Security: Exclude LIFELOG data directory if nested inside watched root
		if (p.dataDir != "" && paths.IsSubpath(p.dataDir, cleanPath)) || (p.canonDataDir != "" && paths.IsSubpath(p.canonDataDir, cleanPath)) {
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Security & Safety: Do NOT follow symlinks recursively
		if d.Type()&os.ModeSymlink != 0 {
			// Check if symlink target resolves into LIFELOG data directory
			if p.dataDir != "" || p.canonDataDir != "" {
				if eval, evalErr := paths.Canonical(cleanPath); evalErr == nil {
					if (p.dataDir != "" && paths.IsSubpath(p.dataDir, eval)) || (p.canonDataDir != "" && paths.IsSubpath(p.canonDataDir, eval)) {
						return nil
					}
				}
			}

			// Record symlink file entry without following target
			info, infoErr := d.Info()
			if infoErr != nil {
				lInfo, lErr := os.Lstat(cleanPath)
				if lErr == nil {
					snap[cleanPath] = fileMeta{
						size:    lInfo.Size(),
						modTime: lInfo.ModTime().UTC().Truncate(time.Millisecond),
						isDir:   false,
					}
				}
				return nil
			}

			snap[cleanPath] = fileMeta{
				size:    info.Size(),
				modTime: info.ModTime().UTC().Truncate(time.Millisecond),
				isDir:   info.IsDir(),
			}
			return nil
		}

		info, infoErr := d.Info()
		if infoErr != nil {
			return nil
		}

		snap[cleanPath] = fileMeta{
			size:    info.Size(),
			modTime: info.ModTime().UTC().Truncate(time.Millisecond),
			isDir:   d.IsDir(),
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	// Preserve prior snapshot entries located under inaccessible directories to avoid false mass deletions
	if len(inaccessibleDirs) > 0 && len(p.snapshot) > 0 {
		for prevPath, prevMeta := range p.snapshot {
			for _, inaccDir := range inaccessibleDirs {
				if prevPath == inaccDir || strings.HasPrefix(prevPath, inaccDir+string(filepath.Separator)) {
					snap[prevPath] = prevMeta
					break
				}
			}
		}
	}

	return snap, nil
}

func (p *FSPoller) diff(oldSnap, newSnap map[string]fileMeta) []events.Event {
	var diffEvents []events.Event
	now := time.Now().UTC()

	// 1. Detect created and modified items
	for path, newMeta := range newSnap {
		oldMeta, exists := oldSnap[path]
		if !exists {
			diffEvents = append(diffEvents, events.Event{
				ID:        nextEventID(),
				Type:      events.EventCreated,
				Path:      path,
				Size:      newMeta.size,
				ModTime:   newMeta.modTime,
				IsDir:     newMeta.isDir,
				Source:    "fs_poll",
				CreatedAt: now,
			})
		} else {
			// Check modification
			if newMeta.size != oldMeta.size || !newMeta.modTime.Equal(oldMeta.modTime) {
				diffEvents = append(diffEvents, events.Event{
					ID:        nextEventID(),
					Type:      events.EventModified,
					Path:      path,
					Size:      newMeta.size,
					ModTime:   newMeta.modTime,
					IsDir:     newMeta.isDir,
					Source:    "fs_poll",
					CreatedAt: now,
				})
			}
		}
	}

	// 2. Detect deleted items
	for path, oldMeta := range oldSnap {
		if _, exists := newSnap[path]; !exists {
			diffEvents = append(diffEvents, events.Event{
				ID:        nextEventID(),
				Type:      events.EventDeleted,
				Path:      path,
				Size:      oldMeta.size,
				ModTime:   now,
				IsDir:     oldMeta.isDir,
				Source:    "fs_poll",
				CreatedAt: now,
			})
		}
	}

	return diffEvents
}
