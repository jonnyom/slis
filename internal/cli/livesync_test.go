package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type failingLiveSyncReader struct {
	err error
}

type diagnosticLiveSyncFailure struct {
	cause error
}

func (failure diagnosticLiveSyncFailure) Error() string {
	return failure.cause.Error()
}

func (failure diagnosticLiveSyncFailure) Unwrap() error {
	return failure.cause
}

func (failure diagnosticLiveSyncFailure) DiagnosticFields() map[string]any {
	return map[string]any{"source_changed_during_mirror": true}
}

func (reader failingLiveSyncReader) Read([]byte) (int, error) {
	return 0, reader.err
}

func TestRunLiveSyncWithInputReturnsInputFailure(t *testing.T) {
	inputFailure := errors.New("input failed")
	err := runLiveSyncWithInput(
		context.Background(),
		failingLiveSyncReader{err: inputFailure},
		func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		},
	)
	if !errors.Is(err, inputFailure) {
		t.Fatalf("runLiveSyncWithInput error = %v", err)
	}
}

func TestRunLiveSyncRecordsFailureForLaterDiagnosis(t *testing.T) {
	cause := errors.New("mirror mismatch")
	failure := diagnosticLiveSyncFailure{cause: cause}
	logPath := filepath.Join(t.TempDir(), "live-sync-crashes.jsonl")
	now := time.Date(2026, time.August, 28, 10, 15, 0, 0, time.UTC)
	err := runLiveSyncWithCrashLog(
		context.Background(),
		strings.NewReader(""),
		logPath,
		func() time.Time { return now },
		func(context.Context) error { return fmt.Errorf("mirror Node-Middleware: %w", failure) },
	)
	if !errors.Is(err, cause) {
		t.Fatalf("runLiveSyncWithCrashLog error = %v", err)
	}
	if !strings.Contains(err.Error(), logPath) {
		t.Fatalf("error does not name crash log: %v", err)
	}
	contents, readErr := os.ReadFile(logPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	var entry liveSyncCrashEntry
	if unmarshalErr := json.Unmarshal([]byte(strings.TrimSpace(string(contents))), &entry); unmarshalErr != nil {
		t.Fatal(unmarshalErr)
	}
	if entry.Timestamp != now.Format(time.RFC3339Nano) || entry.Error != "mirror Node-Middleware: mirror mismatch" {
		t.Fatalf("crash entry = %#v", entry)
	}
	if entry.Diagnostics["source_changed_during_mirror"] != true {
		t.Fatalf("crash diagnostics = %#v", entry.Diagnostics)
	}
	info, statErr := os.Stat(logPath)
	if statErr != nil {
		t.Fatal(statErr)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("crash log mode = %o", info.Mode().Perm())
	}
}
