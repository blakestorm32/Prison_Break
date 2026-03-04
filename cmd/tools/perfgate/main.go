package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"prison-break/internal/perf/gate"
)

type budgetFlagValues []string

func (b *budgetFlagValues) String() string {
	return strings.Join(*b, ",")
}

func (b *budgetFlagValues) Set(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fmt.Errorf("empty budget value")
	}
	*b = append(*b, trimmed)
	return nil
}

func main() {
	var (
		inputPath string
		budgets   budgetFlagValues
	)
	flag.StringVar(&inputPath, "in", "", "path to benchmark output text")
	flag.Var(&budgets, "budget", "benchmark budget in name=max_ns_per_op format (repeatable)")
	flag.Parse()

	if strings.TrimSpace(inputPath) == "" {
		log.Fatal("missing -in benchmark output path")
	}

	raw, err := os.ReadFile(inputPath)
	if err != nil {
		log.Fatalf("read benchmark output: %v", err)
	}
	results := gate.ParseBenchmarkOutput(string(raw))
	checks, parseErr := parseBudgets(budgets)
	if parseErr != nil {
		log.Fatalf("parse budgets: %v", parseErr)
	}

	missing, failures, evalErr := gate.EvaluateBudgets(results, checks)
	if evalErr != nil {
		log.Fatalf("evaluate performance budgets: %v", evalErr)
	}

	if len(missing) > 0 {
		for _, name := range missing {
			log.Printf("missing benchmark result for budget %s", name)
		}
	}
	if len(failures) > 0 {
		for _, failure := range failures {
			log.Printf("budget failure: %s", failure)
		}
	}
	if len(missing) > 0 || len(failures) > 0 {
		os.Exit(1)
	}

	log.Printf("perf gate passed for %d benchmarks", len(checks))
}

func parseBudgets(values []string) ([]gate.BudgetCheck, error) {
	checks := make([]gate.BudgetCheck, 0, len(values))
	for _, value := range values {
		pair := strings.SplitN(strings.TrimSpace(value), "=", 2)
		if len(pair) != 2 {
			return nil, fmt.Errorf("invalid budget %q: expected name=max_ns_per_op", value)
		}
		name := strings.TrimSpace(pair[0])
		if name == "" {
			return nil, fmt.Errorf("invalid budget %q: empty benchmark name", value)
		}
		maxNs, err := strconv.ParseFloat(strings.TrimSpace(pair[1]), 64)
		if err != nil || maxNs <= 0 {
			return nil, fmt.Errorf("invalid budget %q: max ns/op must be > 0", value)
		}
		checks = append(checks, gate.BudgetCheck{
			Name:       name,
			MaxNsPerOp: maxNs,
		})
	}
	return checks, nil
}
