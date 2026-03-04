package gate

import (
	"strings"
	"testing"
)

func TestParseBenchmarkOutputParsesCanonicalNamesAndNsPerOp(t *testing.T) {
	output := strings.Join([]string{
		"goos: windows",
		"goarch: amd64",
		"BenchmarkSnapshotStoreApplyDelta-16     1000000      420.50 ns/op    320 B/op     5 allocs/op",
		"BenchmarkMatchmakingRegionAllocation-16   250000     9800 ns/op       0 B/op      0 allocs/op",
		"PASS",
	}, "\n")

	results := ParseBenchmarkOutput(output)
	if len(results) != 2 {
		t.Fatalf("expected two parsed benchmark results, got %d", len(results))
	}
	if results["SnapshotStoreApplyDelta"].NsPerOp != 420.50 {
		t.Fatalf("expected SnapshotStoreApplyDelta ns/op 420.50, got %+v", results["SnapshotStoreApplyDelta"])
	}
	if results["MatchmakingRegionAllocation"].Iterations != 250000 {
		t.Fatalf("expected MatchmakingRegionAllocation iterations 250000, got %+v", results["MatchmakingRegionAllocation"])
	}
}

func TestEvaluateBudgetsReturnsMissingAndFailures(t *testing.T) {
	results := map[string]BenchmarkResult{
		"SnapshotStoreApplyDelta": {
			Name:       "SnapshotStoreApplyDelta",
			NsPerOp:    600,
			Iterations: 1000,
		},
	}
	missing, failures, err := EvaluateBudgets(results, []BudgetCheck{
		{Name: "SnapshotStoreApplyDelta", MaxNsPerOp: 500},
		{Name: "MatchmakingRegionAllocation", MaxNsPerOp: 10000},
	})
	if err != nil {
		t.Fatalf("evaluate budgets returned unexpected error: %v", err)
	}
	if len(missing) != 1 || missing[0] != "MatchmakingRegionAllocation" {
		t.Fatalf("expected missing benchmark MatchmakingRegionAllocation, got %+v", missing)
	}
	if len(failures) != 1 || !strings.Contains(failures[0], "exceeded budget") {
		t.Fatalf("expected one budget failure message, got %+v", failures)
	}
}

func TestEvaluateBudgetsRejectsInvalidBudgets(t *testing.T) {
	_, _, err := EvaluateBudgets(nil, nil)
	if err == nil {
		t.Fatalf("expected error for empty budgets")
	}

	_, _, err = EvaluateBudgets(nil, []BudgetCheck{{Name: "bad", MaxNsPerOp: 0}})
	if err == nil {
		t.Fatalf("expected error for invalid budget threshold")
	}
}
