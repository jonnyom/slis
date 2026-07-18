package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/rpcserver"
)

var rpcCmd = &cobra.Command{
	Use:   "rpc",
	Short: "[experimental] Run a read-only JSON-RPC sidecar over stdio (for the JS TUI)",
	Long: "Run a long-lived, strictly read-only JSON-RPC 2.0 server over stdin/stdout\n" +
		"with NDJSON framing (one JSON object per line). It reuses the same read\n" +
		"builders as the --json commands and pushes sessionEvent notifications when a\n" +
		"slice's Claude session status changes. Experimental: the RPC surface is v0.",
	RunE: func(cmd *cobra.Command, args []string) error {
		ws, err := config.LoadWorkspace(config.WorkspacePath())
		if err != nil {
			return fmt.Errorf("workspace not found — run `slis init` first: %w", err)
		}

		sp := config.StatePaths()
		// Ensure the events dir exists so the session-status watcher can attach on a
		// fresh workspace (idempotent).
		_ = sp.EnsureDirs()

		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()

		srv := rpcserver.New(ws, sp, Version)
		return srv.Serve(ctx, os.Stdin, os.Stdout)
	},
}

func init() {
	rootCmd.AddCommand(rpcCmd)
}
