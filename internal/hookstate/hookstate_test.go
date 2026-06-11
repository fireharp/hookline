package hookstate

import (
	"testing"
	"time"
)

func TestActiveSnoozeMatchesPathSessionAndProjectWildcard(t *testing.T) {
	store := New(t.TempDir())
	now := time.Now().UTC()
	_, err := store.AddSnooze(Snooze{
		RuleID:    "large-file-split-review",
		Path:      "huge.ts",
		Scope:     ScopeSession,
		SessionID: "s1",
		ExpiresAt: now.Add(time.Hour).Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.AddSnooze(Snooze{
		RuleID:    "large-file-split-review",
		Path:      "*",
		Scope:     ScopeProject,
		ExpiresAt: now.Add(time.Hour).Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatal(err)
	}
	if snooze, err := store.ActiveSnooze("large-file-split-review", "huge.ts", "s1", now); err != nil || snooze == nil || snooze.Scope != ScopeProject {
		t.Fatalf("expected latest project wildcard snooze, got %#v err=%v", snooze, err)
	}
	if snooze, err := store.ActiveSnooze("large-file-split-review", "other.ts", "s2", now); err != nil || snooze == nil || snooze.Path != "*" {
		t.Fatalf("expected project wildcard to match other session/path, got %#v err=%v", snooze, err)
	}
}

func TestExpiredSnoozeDoesNotMatch(t *testing.T) {
	store := New(t.TempDir())
	now := time.Now().UTC()
	_, err := store.AddSnooze(Snooze{
		RuleID:    "large-file-split-review",
		Path:      "huge.ts",
		Scope:     ScopeProject,
		ExpiresAt: now.Add(-time.Minute).Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatal(err)
	}
	snooze, err := store.ActiveSnooze("large-file-split-review", "huge.ts", "", now)
	if err != nil {
		t.Fatal(err)
	}
	if snooze != nil {
		t.Fatalf("expected expired snooze not to match, got %#v", snooze)
	}
}

func TestLatestDecisionReturnsMostRecentMatchingEntry(t *testing.T) {
	store := New(t.TempDir())
	if _, err := store.AddDecision(Decision{RuleID: "large-file-split-review", Path: "huge.ts", Action: "keep", Result: "old"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddDecision(Decision{RuleID: "large-file-split-review", Path: "huge.ts", Action: "split", Result: "new"}); err != nil {
		t.Fatal(err)
	}
	decision, err := store.LatestDecision("large-file-split-review", "huge.ts")
	if err != nil {
		t.Fatal(err)
	}
	if decision == nil || decision.Action != "split" || decision.Result != "new" {
		t.Fatalf("expected latest split decision, got %#v", decision)
	}
}
