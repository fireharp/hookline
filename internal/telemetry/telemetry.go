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
	Event             string `json:"event,omitempty"`
	Tool              string `json:"tool,omitempty"`
	Decision          string `json:"decision,omitempty"`
	Reason            string `json:"reason,omitempty"`
	AdditionalContext string `json:"additional_context,omitempty"`
	OutputEmpty       bool   `json:"output_empty"`
	Error             string `json:"error,omitempty"`
	DurationMS        int64  `json:"duration_ms"`
}

func Append(root string, cfg config.Config, input, output []byte, hookErr error, duration time.Duration) error {
	if !config.TelemetryEnabled(cfg) {
		return nil
	}
	record := BuildRecord(root, input, output, hookErr, duration)
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

func BuildRecord(root string, input, output []byte, hookErr error, duration time.Duration) Record {
	record := Record{
		Time:        time.Now().UTC().Format(time.RFC3339Nano),
		Root:        root,
		OutputEmpty: len(output) == 0,
		DurationMS:  duration.Milliseconds(),
	}
	var raw map[string]any
	if err := json.Unmarshal(input, &raw); err == nil {
		record.CWD = stringField(raw, "cwd")
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
