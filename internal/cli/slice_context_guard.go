package cli

import (
	"fmt"
	"os"
	"strings"
)

func managedSliceMutationGuard(allowFromSlice bool, operation string) error {
	currentSlice := strings.TrimSpace(os.Getenv("SLIS_SLICE"))
	if currentSlice == "" || allowFromSlice {
		return nil
	}
	return fmt.Errorf(
		"refusing to %s while running inside slice %q; agent scratch worktrees belong under .claude/worktrees, not in the managed slice registry; pass --allow-from-slice to create a separate managed slice intentionally",
		operation,
		currentSlice,
	)
}
