package cli

import (
	"errors"
	"fmt"
	"io"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/swap"
	"github.com/spf13/cobra"
)

func recoverActivationDrift(journalPath string, failure error, output io.Writer) error {
	if !errors.Is(failure, swap.ErrUnknownPrimaryChanges) {
		return failure
	}
	recoveryPath, err := swap.ArchiveDriftedActivation(journalPath)
	if err != nil {
		return errors.Join(failure, err)
	}
	if recoveryPath == "" {
		return failure
	}
	return reportArchivedActivation(recoveryPath, output)
}

func reportArchivedActivation(recoveryPath string, output io.Writer) error {
	if recoveryPath == "" {
		return nil
	}
	_, err := fmt.Fprintf(output, "Live slice deactivated because its checkout changed outside Slis. Your files and branches were left unchanged. Recovery record: %s\n", recoveryPath)
	return err
}

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:    "recover-activation",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			recoveryPath, err := swap.ArchiveDriftedActivation(config.StatePaths().ActiveJournal)
			if err != nil {
				return err
			}
			return reportArchivedActivation(recoveryPath, cmd.ErrOrStderr())
		},
	})
}
