package hookstate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fireharp/hookline/internal/types"
)

type DedupeEntry struct {
	Time   string `json:"time"`
	Source string `json:"source,omitempty"`
	Event  string `json:"event,omitempty"`
}

func (s Store) Acquire(event types.Event, input []byte, source string, ttl time.Duration) (bool, error) {
	if ttl <= 0 {
		ttl = 2 * time.Minute
	}
	dir := filepath.Join(s.Root, ".harness", "locks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false, err
	}
	s.cleanupLocks(dir, ttl)
	path := filepath.Join(dir, s.dedupeKey(event, input)+".json")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if errors.Is(err, os.ErrExist) {
		if stale(path, ttl) {
			_ = os.Remove(path)
			file, err = os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		}
	}
	if errors.Is(err, os.ErrExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer file.Close()
	_ = json.NewEncoder(file).Encode(DedupeEntry{
		Time:   time.Now().UTC().Format(time.RFC3339Nano),
		Source: source,
		Event:  event.Event,
	})
	return true, nil
}

func (s Store) dedupeKey(event types.Event, input []byte) string {
	hash := sha256.Sum256(input)
	parts := []string{
		event.SessionID,
		event.TurnID,
		event.Event,
		"",
		event.PermissionMode,
		hex.EncodeToString(hash[:12]),
	}
	if event.Tool != nil {
		parts[3] = event.Tool.Name
	}
	if event.StopHookActive {
		parts = append(parts, "stop-active")
	}
	key := strings.Join(parts, "|")
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

func (s Store) cleanupLocks(dir string, ttl time.Duration) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if stale(path, ttl) {
			_ = os.Remove(path)
		}
	}
}

func stale(path string, ttl time.Duration) bool {
	info, err := os.Stat(path)
	return err == nil && time.Since(info.ModTime()) > ttl
}
