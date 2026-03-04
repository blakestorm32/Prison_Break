package crashreport

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const defaultCrashReportDir = "build/release/crash_reports"

var nowFn = time.Now

func ReportRecoveredPanic(appName string, recovered any, stack []byte) bool {
	if recovered == nil {
		return false
	}
	if len(stack) == 0 {
		stack = []byte("stack unavailable")
	}

	path, err := WritePanicReport(
		appName,
		recovered,
		stack,
		nowFn().UTC(),
		DefaultOutputDir(),
	)
	if err != nil {
		log.Printf("panic recovered for %s but crash report write failed: %v", strings.TrimSpace(appName), err)
		return true
	}

	log.Printf("panic recovered for %s; crash report written to %s", strings.TrimSpace(appName), path)
	return true
}

func DefaultOutputDir() string {
	fromEnv := strings.TrimSpace(os.Getenv("PRISON_CRASH_REPORT_DIR"))
	if fromEnv != "" {
		return fromEnv
	}
	return defaultCrashReportDir
}

func WritePanicReport(
	appName string,
	recovered any,
	stack []byte,
	occurredAt time.Time,
	outputDir string,
) (string, error) {
	normalizedApp := strings.TrimSpace(appName)
	if normalizedApp == "" {
		normalizedApp = "app"
	}
	normalizedDir := strings.TrimSpace(outputDir)
	if normalizedDir == "" {
		normalizedDir = defaultCrashReportDir
	}

	if err := os.MkdirAll(normalizedDir, 0o755); err != nil {
		return "", fmt.Errorf("create crash report directory: %w", err)
	}

	filename := fmt.Sprintf(
		"%s_%s.log",
		sanitizeFileToken(normalizedApp),
		occurredAt.UTC().Format("20060102_150405"),
	)
	path := filepath.Join(normalizedDir, filename)
	body := fmt.Sprintf(
		"app=%s\noccurred_at=%s\npanic=%v\n\nstack:\n%s\n",
		normalizedApp,
		occurredAt.UTC().Format(time.RFC3339Nano),
		recovered,
		string(stack),
	)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return "", fmt.Errorf("write crash report: %w", err)
	}
	return path, nil
}

func sanitizeFileToken(value string) string {
	if value == "" {
		return "app"
	}
	out := make([]rune, 0, len(value))
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			out = append(out, r)
		case r >= 'A' && r <= 'Z':
			out = append(out, r)
		case r >= '0' && r <= '9':
			out = append(out, r)
		case r == '-' || r == '_':
			out = append(out, r)
		default:
			out = append(out, '_')
		}
	}
	return string(out)
}
