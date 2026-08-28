package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/swap"
	"github.com/spf13/cobra"
)

var liveSyncCmd = &cobra.Command{
	Use:    "live-sync",
	Hidden: true,
	Args:   cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer cancel()
		paths := config.StatePaths()
		return runLiveSyncWithCrashLog(
			ctx,
			os.Stdin,
			filepath.Join(paths.StateDir, "live-sync-crashes.jsonl"),
			time.Now,
			func(syncContext context.Context) error {
				return swap.RunLiveSync(syncContext, paths.ActiveJournal, 30*time.Second)
			},
		)
	},
}

type liveSyncCrashEntry struct {
	Timestamp   string         `json:"timestamp"`
	Error       string         `json:"error"`
	Diagnostics map[string]any `json:"diagnostics,omitempty"`
}

type liveSyncDiagnosticError interface {
	DiagnosticFields() map[string]any
}

func runLiveSyncWithCrashLog(
	ctx context.Context,
	input io.Reader,
	logPath string,
	now func() time.Time,
	run func(context.Context) error,
) error {
	failure := runLiveSyncWithInput(ctx, input, run)
	if failure == nil {
		return nil
	}
	entry := liveSyncCrashEntry{Timestamp: now().Format(time.RFC3339Nano), Error: failure.Error()}
	var diagnosticFailure liveSyncDiagnosticError
	if errors.As(failure, &diagnosticFailure) {
		entry.Diagnostics = diagnosticFailure.DiagnosticFields()
	}
	if err := appendLiveSyncCrash(logPath, entry); err != nil {
		return errors.Join(failure, fmt.Errorf("write live sync crash log %q: %w", logPath, err))
	}
	return fmt.Errorf("%w\nlive sync crash log: %s", failure, logPath)
}

func appendLiveSyncCrash(path string, entry liveSyncCrashEntry) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	contents, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(append(contents, '\n'))
	return errors.Join(writeErr, file.Close())
}

func runLiveSyncWithInput(ctx context.Context, input io.Reader, run func(context.Context) error) error {
	syncContext, cancel := context.WithCancel(ctx)
	defer cancel()
	inputResult := make(chan error, 1)
	go func() {
		_, err := io.Copy(io.Discard, input)
		inputResult <- err
		cancel()
	}()
	syncErr := run(syncContext)
	select {
	case inputErr := <-inputResult:
		if inputErr != nil {
			return inputErr
		}
	default:
	}
	return syncErr
}

func init() {
	rootCmd.AddCommand(liveSyncCmd)
}
