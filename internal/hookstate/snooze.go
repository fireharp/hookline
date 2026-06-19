package hookstate

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

const (
	ScopeSession = "session"
	ScopeProject = "project"
)

type Store struct {
	Root string
}

type Snooze struct {
	ID        string `json:"id"`
	Time      string `json:"time"`
	RuleID    string `json:"rule_id"`
	Path      string `json:"path"`
	Scope     string `json:"scope"`
	SessionID string `json:"session_id,omitempty"`
	ExpiresAt string `json:"expires_at"`
	Reason    string `json:"reason,omitempty"`
}

type SnoozeFilter struct {
	RuleID    string
	Path      string
	Scope     string
	SessionID string
	All       bool
}

func New(root string) Store {
	return Store{Root: root}
}

func (s Store) AddSnooze(entry Snooze) (Snooze, error) {
	if entry.ID == "" {
		entry.ID = randomID()
	}
	if entry.Time == "" {
		entry.Time = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if entry.Path == "" {
		entry.Path = "*"
	}
	if entry.Scope == "" {
		entry.Scope = ScopeSession
	}
	return entry, appendJSONL(s.snoozePath(), entry)
}

func (s Store) Snoozes() ([]Snooze, error) {
	out := []Snooze{}
	err := readJSONL(s.snoozePath(), func(data []byte) {
		var entry Snooze
		if json.Unmarshal(data, &entry) == nil {
			out = append(out, entry)
		}
	})
	return out, err
}

func (s Store) ActiveSnooze(ruleID, path, sessionID string, now time.Time) (*Snooze, error) {
	entries, err := s.Snoozes()
	if err != nil {
		return nil, err
	}
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		if entry.RuleID != ruleID || !pathMatches(entry.Path, path) {
			continue
		}
		if entry.Scope == ScopeSession && (sessionID == "" || entry.SessionID != sessionID) {
			continue
		}
		if expired(entry.ExpiresAt, now) {
			continue
		}
		return &entry, nil
	}
	return nil, nil
}

func (s Store) ClearSnoozes(filter SnoozeFilter) (int, error) {
	entries, err := s.Snoozes()
	if err != nil {
		return 0, err
	}
	var keep []Snooze
	removed := 0
	for _, entry := range entries {
		if filter.All || filter.match(entry) {
			removed++
			continue
		}
		keep = append(keep, entry)
	}
	return removed, rewriteJSONL(s.snoozePath(), keep)
}

func (s Store) ActiveSnoozeCount(now time.Time) (int, error) {
	entries, err := s.Snoozes()
	if err != nil {
		return 0, err
	}
	count := 0
	for _, entry := range entries {
		if !expired(entry.ExpiresAt, now) {
			count++
		}
	}
	return count, nil
}

func (f SnoozeFilter) match(entry Snooze) bool {
	if f.RuleID != "" && f.RuleID != entry.RuleID {
		return false
	}
	if f.Path != "" && f.Path != entry.Path {
		return false
	}
	if f.Scope != "" && f.Scope != entry.Scope {
		return false
	}
	if f.SessionID != "" && f.SessionID != entry.SessionID {
		return false
	}
	return f.RuleID != "" || f.Path != "" || f.Scope != "" || f.SessionID != ""
}

func pathMatches(pattern, path string) bool {
	return pattern == "*" || pattern == path
}

func expired(value string, now time.Time) bool {
	t, err := time.Parse(time.RFC3339Nano, value)
	return err == nil && !t.After(now)
}

func (s Store) snoozePath() string {
	return filepath.Join(s.Root, ".harness", "snoozes.jsonl")
}

func appendJSONL(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	return json.NewEncoder(file).Encode(value)
}

func readJSONL(path string, visit func([]byte)) error {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		visit(scanner.Bytes())
	}
	return scanner.Err()
}

func rewriteJSONL[T any](path string, values []T) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(file)
	for _, value := range values {
		if err := enc.Encode(value); err != nil {
			_ = file.Close()
			return err
		}
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func randomID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return time.Now().UTC().Format("20060102150405.000000000")
	}
	return hex.EncodeToString(b[:])
}
