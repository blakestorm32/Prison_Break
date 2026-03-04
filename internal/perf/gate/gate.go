package gate

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type BenchmarkResult struct {
	Name       string  `json:"name"`
	NsPerOp    float64 `json:"ns_per_op"`
	Iterations int64   `json:"iterations"`
}

type BudgetCheck struct {
	Name       string  `json:"name"`
	MaxNsPerOp float64 `json:"max_ns_per_op"`
}

func ParseBenchmarkOutput(output string) map[string]BenchmarkResult {
	lines := strings.Split(output, "\n")
	results := make(map[string]BenchmarkResult, len(lines))

	for _, line := range lines {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 4 {
			continue
		}
		if !strings.HasPrefix(fields[0], "Benchmark") {
			continue
		}

		name := canonicalBenchmarkName(fields[0])
		if name == "" {
			continue
		}

		iterations, iterErr := strconv.ParseInt(fields[1], 10, 64)
		if iterErr != nil {
			continue
		}

		nsValue, ok := parseNsPerOp(fields)
		if !ok {
			continue
		}

		results[name] = BenchmarkResult{
			Name:       name,
			NsPerOp:    nsValue,
			Iterations: iterations,
		}
	}

	return results
}

func EvaluateBudgets(
	results map[string]BenchmarkResult,
	budgets []BudgetCheck,
) (missing []string, failures []string, err error) {
	if len(budgets) == 0 {
		return nil, nil, errors.New("no performance budgets provided")
	}

	missing = make([]string, 0, len(budgets))
	failures = make([]string, 0, len(budgets))
	for _, budget := range budgets {
		name := strings.TrimSpace(budget.Name)
		if name == "" || budget.MaxNsPerOp <= 0 {
			return nil, nil, fmt.Errorf("invalid budget: %+v", budget)
		}

		result, exists := results[name]
		if !exists {
			missing = append(missing, name)
			continue
		}
		if result.NsPerOp > budget.MaxNsPerOp {
			failures = append(
				failures,
				fmt.Sprintf(
					"%s exceeded budget: %.2f ns/op > %.2f ns/op",
					name,
					result.NsPerOp,
					budget.MaxNsPerOp,
				),
			)
		}
	}

	return missing, failures, nil
}

func parseNsPerOp(fields []string) (float64, bool) {
	for index := 2; index < len(fields); index++ {
		if fields[index] != "ns/op" || index == 0 {
			continue
		}
		value, err := strconv.ParseFloat(fields[index-1], 64)
		if err != nil {
			return 0, false
		}
		return value, true
	}
	return 0, false
}

func canonicalBenchmarkName(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, "Benchmark") {
		return ""
	}

	name := strings.TrimPrefix(trimmed, "Benchmark")
	if name == "" {
		return ""
	}
	lastDash := strings.LastIndex(name, "-")
	if lastDash == -1 || lastDash == len(name)-1 {
		return name
	}
	suffix := name[lastDash+1:]
	if _, err := strconv.Atoi(suffix); err == nil {
		return name[:lastDash]
	}
	return name
}
