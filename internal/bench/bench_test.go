package bench

import (
	"context"
	"testing"

	"github.com/fireharp/hookline/internal/config"
	"github.com/fireharp/hookline/internal/recipes"
)

func TestSmokeBenchPasses(t *testing.T) {
	cfg := config.Default()
	cfg.Recipes.Enabled = []string{recipes.LineCount, recipes.AgentSteering}
	result, err := Run(context.Background(), "smoke", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Pass {
		t.Fatalf("expected pass, got %#v", result)
	}
	if len(result.Scenarios) != 6 {
		t.Fatalf("expected six scenarios, got %d", len(result.Scenarios))
	}
	for _, scenario := range result.Scenarios {
		if !scenario.Resolved {
			t.Fatalf("expected %s to be resolved", scenario.Name)
		}
		if scenario.Start == "" || scenario.Hookline == "" || scenario.AgentAction == "" || scenario.Result == "" {
			t.Fatalf("expected %s to describe start, hookline, agent action, and result: %#v", scenario.Name, scenario)
		}
		if scenario.Evidence.CaseID == "" || scenario.Evidence.Project == "" || scenario.Evidence.Rule == "" {
			t.Fatalf("expected %s to include case metadata: %#v", scenario.Name, scenario.Evidence)
		}
		if len(scenario.Evidence.Communication) == 0 || scenario.Evidence.Hook.Event == "" || scenario.Evidence.Hook.Output == "" {
			t.Fatalf("expected %s to include replay communication and hook output: %#v", scenario.Name, scenario.Evidence)
		}
		if len(scenario.Evidence.InitialState) == 0 || len(scenario.Evidence.FinalState) == 0 || len(scenario.Evidence.Verification) == 0 {
			t.Fatalf("expected %s to include state and verification evidence: %#v", scenario.Name, scenario.Evidence)
		}
	}
}
