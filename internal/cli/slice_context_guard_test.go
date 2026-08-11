package cli

import (
	"strings"
	"testing"
)

func TestManagedSliceMutationGuardRejectsAgentCreatingAnotherSlice(t *testing.T) {
	t.Setenv("SLIS_SLICE", "unpaid-leave")

	err := managedSliceMutationGuard(false, "create a managed slice")

	if err == nil {
		t.Fatal("expected managed slice mutation to be rejected")
	}
	if !strings.Contains(err.Error(), `inside slice "unpaid-leave"`) {
		t.Fatalf("error = %q, want current slice", err)
	}
	if !strings.Contains(err.Error(), "--allow-from-slice") {
		t.Fatalf("error = %q, want explicit override", err)
	}
}

func TestManagedSliceMutationGuardAllowsExplicitOverride(t *testing.T) {
	t.Setenv("SLIS_SLICE", "unpaid-leave")

	if err := managedSliceMutationGuard(true, "create a managed slice"); err != nil {
		t.Fatalf("explicit override: %v", err)
	}
}

func TestManagedSliceMutationGuardAllowsTopLevelCommand(t *testing.T) {
	t.Setenv("SLIS_SLICE", "")

	if err := managedSliceMutationGuard(false, "create a managed slice"); err != nil {
		t.Fatalf("top-level command: %v", err)
	}
}

func TestManagedSliceCommandsRejectAgentContext(t *testing.T) {
	t.Setenv("SLIS_SLICE", "unpaid-leave")
	commands := []struct {
		name string
		run  func() error
	}{
		{
			name: "create",
			run: func() error {
				return createCmd.RunE(createCmd, []string{"unpaid-leave-punchable-guard"})
			},
		},
		{
			name: "adopt",
			run: func() error {
				return adoptCmd.RunE(adoptCmd, []string{"unpaid-leave-punchable-guard"})
			},
		},
		{
			name: "import",
			run: func() error {
				return importCmd.RunE(importCmd, []string{"/tmp/agent-worktree"})
			},
		},
	}

	for _, command := range commands {
		t.Run(command.name, func(t *testing.T) {
			err := command.run()
			if err == nil || !strings.Contains(err.Error(), `inside slice "unpaid-leave"`) {
				t.Fatalf("error = %v, want active-slice rejection", err)
			}
		})
	}
}
