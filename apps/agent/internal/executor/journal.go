package executor

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	journalMaxEntries = 2048
	journalFileName   = "command-journal.json"
)

// journalEntry represents a single persisted command record.
type journalEntry struct {
	ID        string    `json:"id"`
	ExpiresAt time.Time `json:"expiresAt"`
	SeenAt    time.Time `json:"seenAt"`
}

// journal provides replay protection for commands. It is safe for concurrent use.
// Entries are held in memory in insertion order and persisted to disk so that
// the agent cannot re-execute the same command across restarts, within the
// command's TTL window.
type journal struct {
	mu      sync.Mutex
	entries map[string]journalEntry // keyed by command ID
	order   []string                // insertion order for eviction
	dataDir string
	log     *slog.Logger
}

func newJournal(dataDir string, log *slog.Logger) *journal {
	return &journal{
		entries: make(map[string]journalEntry, journalMaxEntries),
		order:   make([]string, 0, journalMaxEntries),
		dataDir: dataDir,
		log:     log,
	}
}

// Load reads the persisted journal from disk, discarding expired entries.
func (j *journal) Load() {
	j.mu.Lock()
	defer j.mu.Unlock()

	path := j.filePath()
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			j.log.Warn("could not read command journal", slog.String("path", path), slog.String("error", err.Error()))
		}
		return
	}

	var entries []journalEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		j.log.Warn("could not parse command journal, starting fresh", slog.String("error", err.Error()))
		return
	}

	now := time.Now()
	for _, e := range entries {
		keep := false
		if e.ExpiresAt.IsZero() {
			// Legacy entries without TTL: retain for 7 days from SeenAt.
			if !e.SeenAt.IsZero() && now.Sub(e.SeenAt) < 7*24*time.Hour {
				keep = true
			}
		} else if e.ExpiresAt.After(now) {
			keep = true
		}
		if keep {
			j.entries[e.ID] = e
			j.order = append(j.order, e.ID)
		}
	}
	j.log.Info("loaded command journal", slog.Int("entries", len(j.entries)))
}

// Has returns true if the command ID has been seen before, indicating a replay.
func (j *journal) Has(id string) bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	_, ok := j.entries[id]
	return ok
}

// Add records a command ID in the journal and persists to disk.
func (j *journal) Add(id string, expiresAt time.Time) {
	j.mu.Lock()
	defer j.mu.Unlock()

	if _, exists := j.entries[id]; exists {
		return
	}

	entry := journalEntry{ID: id, ExpiresAt: expiresAt, SeenAt: time.Now()}
	j.entries[id] = entry
	j.order = append(j.order, id)

	// Evict oldest entries when over capacity
	for len(j.order) > journalMaxEntries {
		oldest := j.order[0]
		j.order = j.order[1:]
		delete(j.entries, oldest)
	}

	j.persistLocked()
}

// Prune removes expired entries and re-persists.
func (j *journal) Prune() {
	j.mu.Lock()
	defer j.mu.Unlock()

	now := time.Now()
	newOrder := j.order[:0]
	for _, id := range j.order {
		e, ok := j.entries[id]
		if !ok {
			continue
		}
		keep := false
		if e.ExpiresAt.IsZero() {
			if !e.SeenAt.IsZero() && now.Sub(e.SeenAt) < 7*24*time.Hour {
				keep = true
			}
		} else if e.ExpiresAt.After(now) {
			keep = true
		}
		if keep {
			newOrder = append(newOrder, id)
		} else {
			delete(j.entries, id)
		}
	}
	j.order = newOrder
	j.persistLocked()
}

func (j *journal) filePath() string {
	return filepath.Join(j.dataDir, journalFileName)
}

// persistLocked writes the journal to disk atomically. Caller must hold j.mu.
func (j *journal) persistLocked() {
	entries := make([]journalEntry, 0, len(j.entries))
	for _, e := range j.entries {
		entries = append(entries, e)
	}
	data, err := json.Marshal(entries)
	if err != nil {
		j.log.Error("failed to marshal command journal", slog.String("error", err.Error()))
		return
	}
	path := j.filePath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		j.log.Error("failed to create journal directory", slog.String("error", err.Error()))
		return
	}

	tmpFile, err := os.CreateTemp(dir, "command-journal-*.tmp")
	if err != nil {
		j.log.Error("failed to create temp journal file", slog.String("error", err.Error()))
		return
	}
	tmpName := tmpFile.Name()
	defer func() {
		if tmpFile != nil {
			_ = tmpFile.Close()
			_ = os.Remove(tmpName)
		}
	}()

	if err := tmpFile.Chmod(0600); err != nil {
		j.log.Error("failed to chmod temp journal file", slog.String("error", err.Error()))
		return
	}

	if _, err := tmpFile.Write(data); err != nil {
		j.log.Error("failed to write to temp journal file", slog.String("error", err.Error()))
		return
	}

	if err := tmpFile.Sync(); err != nil {
		j.log.Error("failed to sync temp journal file", slog.String("error", err.Error()))
		return
	}

	if err := tmpFile.Close(); err != nil {
		j.log.Error("failed to close temp journal file", slog.String("error", err.Error()))
		return
	}
	tmpFile = nil

	if err := os.Rename(tmpName, path); err != nil {
		j.log.Error("failed to rename temp journal file", slog.String("path", path), slog.String("error", err.Error()))
	}
}
