package bench

import (
	"context"
	"testing"

	"github.com/fireharp/hookline/internal/config"
)

func TestSmokeBenchPasses(t *testing.T) {
	result, err := Run(context.Background(), "smoke", config.Default())
	if err != nil {
		t.Fatal(err)
	}
	if !result.Pass {
		t.Fatalf("expected pass, got %#v", result)
	}
	if len(result.Scenarios) != 5 {
		t.Fatalf("expected five scenarios, got %d", len(result.Scenarios))
	}
}
