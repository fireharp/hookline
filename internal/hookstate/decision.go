package hookstate

import (
	"encoding/json"
	"path/filepath"
	"time"
)

type Decision struct {
	Time      string `json:"time"`
	RuleID    string `json:"rule_id"`
	Path      string `json:"path"`
	Action    string `json:"action"`
	SessionID string `json:"session_id,omitempty"`
	WhySplit  string `json:"why_split,omitempty"`
	WhyNotNow string `json:"why_not_now,omitempty"`
	Result    string `json:"result,omitempty"`
}

func (s Store) AddDecision(entry Decision) (Decision, error) {
	if entry.Time == "" {
		entry.Time = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if entry.Path == "" {
		entry.Path = "*"
	}
	return entry, appendJSONL(s.decisionPath(), entry)
}

func (s Store) Decisions() ([]Decision, error) {
	out := []Decision{}
	err := readJSONL(s.decisionPath(), func(data []byte) {
		var entry Decision
		if json.Unmarshal(data, &entry) == nil {
			out = append(out, entry)
		}
	})
	return out, err
}

func (s Store) LatestDecision(ruleID, path string) (*Decision, error) {
	entries, err := s.Decisions()
	if err != nil {
		return nil, err
	}
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		if entry.RuleID == ruleID && (entry.Path == path || entry.Path == "*") {
			return &entry, nil
		}
	}
	return nil, nil
}

func (s Store) decisionPath() string {
	return filepath.Join(s.Root, ".hookline", "decisions.jsonl")
}
