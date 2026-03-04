package crashreport

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWritePanicReportWritesStructuredFile(t *testing.T) {
	dir := t.TempDir()
	path, err := WritePanicReport(
		"server",
		"boom",
		[]byte("stack line 1\nstack line 2"),
		time.Date(2026, 3, 4, 12, 34, 56, 0, time.UTC),
		dir,
	)
	if err != nil {
		t.Fatalf("write panic report failed: %v", err)
	}

	if !strings.Contains(path, filepath.Join(dir, "server_20260304_123456.log")) {
		t.Fatalf("unexpected crash report path %s", path)
	}
	raw, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read panic report file: %v", readErr)
	}
	body := string(raw)
	if !strings.Contains(body, "app=server") {
		t.Fatalf("expected app field in crash report body, got %q", body)
	}
	if !strings.Contains(body, "panic=boom") {
		t.Fatalf("expected panic message in crash report body, got %q", body)
	}
	if !strings.Contains(body, "stack line 1") {
		t.Fatalf("expected stack trace in crash report body, got %q", body)
	}
}

func TestRecoverAndReportCapturesPanicToConfiguredDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PRISON_CRASH_REPORT_DIR", dir)

	originalNow := nowFn
	nowFn = func() time.Time {
		return time.Date(2026, 3, 4, 14, 0, 0, 0, time.UTC)
	}
	defer func() {
		nowFn = originalNow
	}()

	recovered := invokeRecoverAndReportForTest("client", "panic-value")
	if !recovered {
		t.Fatalf("expected recover helper to report recovered panic")
	}

	matches, err := filepath.Glob(filepath.Join(dir, "client_20260304_140000.log"))
	if err != nil {
		t.Fatalf("glob crash report path: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected one crash report file, got %d (matches=%v)", len(matches), matches)
	}
}

func invokeRecoverAndReportForTest(appName string, panicValue any) (recovered bool) {
	defer func() {
		recovered = ReportRecoveredPanic(appName, recover(), []byte("test-stack"))
	}()
	panic(panicValue)
}
