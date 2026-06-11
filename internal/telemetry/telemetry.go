package telemetry

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/fireharp/hookline/internal/config"
)

type Record struct {
	Time              string `json:"time"`
	Root              string `json:"root"`
	CWD               string `json:"cwd,omitempty"`
	SessionID         string `json:"session_id,omitempty"`
	TurnID            string `json:"turn_id,omitempty"`
	Event             string `json:"event,omitempty"`
	Tool              string `json:"tool,omitempty"`
	Decision          string `json:"decision,omitempty"`
	RuleID            string `json:"rule_id,omitempty"`
	Source            string `json:"source,omitempty"`
	Deduped           bool   `json:"deduped,omitempty"`
	Snoozed           bool   `json:"snoozed,omitempty"`
	SnoozeScope       string `json:"snooze_scope,omitempty"`
	SnoozePath        string `json:"snooze_path,omitempty"`
	Reason            string `json:"reason,omitempty"`
	AdditionalContext string `json:"additional_context,omitempty"`
	OutputEmpty       bool   `json:"output_empty"`
	Error             string `json:"error,omitempty"`
	DurationMS        int64  `json:"duration_ms"`
}

type Meta struct {
	RuleID      string
	Source      string
	Deduped     bool
	Snoozed     bool
	SnoozeScope string
	SnoozePath  string
}

func Append(root string, cfg config.Config, input, output []byte, hookErr error, duration time.Duration, meta ...Meta) error {
	if !config.TelemetryEnabled(cfg) {
		return nil
	}
	record := BuildRecord(root, input, output, hookErr, duration, meta...)
	path := Path(root, cfg.Telemetry)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	return json.NewEncoder(file).Encode(record)
}

func BuildRecord(root string, input, output []byte, hookErr error, duration time.Duration, meta ...Meta) Record {
	record := Record{
		Time:        time.Now().UTC().Format(time.RFC3339Nano),
		Root:        root,
		OutputEmpty: len(output) == 0,
		DurationMS:  duration.Milliseconds(),
	}
	if len(meta) > 0 {
		record.RuleID = meta[0].RuleID
		record.Source = meta[0].Source
		record.Deduped = meta[0].Deduped
		record.Snoozed = meta[0].Snoozed
		record.SnoozeScope = meta[0].SnoozeScope
		record.SnoozePath = meta[0].SnoozePath
	}
	var raw map[string]any
	if err := json.Unmarshal(input, &raw); err == nil {
		record.CWD = stringField(raw, "cwd")
		record.SessionID = stringField(raw, "session_id")
		record.TurnID = stringField(raw, "turn_id")
		record.Event = stringField(raw, "hook_event_name")
		record.Tool = stringField(raw, "tool_name")
	}
	if len(output) > 0 {
		var out map[string]any
		if err := json.Unmarshal(output, &out); err == nil {
			record.Decision = stringField(out, "decision")
			record.Reason = stringField(out, "reason")
			record.AdditionalContext = stringField(out, "systemMessage")
			if nested, ok := out["hookSpecificOutput"].(map[string]any); ok {
				if record.AdditionalContext == "" {
					record.AdditionalContext = stringField(nested, "additionalContext")
				}
				if record.Reason == "" {
					record.Reason = stringField(nested, "permissionDecisionReason")
				}
				if record.Decision == "" {
					record.Decision = stringField(nested, "permissionDecision")
				}
			}
		}
	}
	if hookErr != nil {
		record.Error = hookErr.Error()
	}
	return record
}

func Tail(root string, cfg config.TelemetryConfig, limit int) ([]Record, error) {
	if limit <= 0 {
		limit = 20
	}
	file, err := os.Open(Path(root, cfg))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var records []Record
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var record Record
		if err := json.Unmarshal(scanner.Bytes(), &record); err == nil {
			records = append(records, record)
		}
		if len(records) > limit {
			copy(records, records[1:])
			records = records[:limit]
		}
	}
	return records, scanner.Err()
}

func Count(root string, cfg config.TelemetryConfig) (int, error) {
	file, err := os.Open(Path(root, cfg))
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer file.Close()
	count := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		count++
	}
	return count, scanner.Err()
}

func Path(root string, cfg config.TelemetryConfig) string {
	path := cfg.Path
	if path == "" {
		path = ".hookline/events.jsonl"
	}
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(root, path)
}

func stringField(raw map[string]any, key string) string {
	value, _ := raw[key].(string)
	return value
}
