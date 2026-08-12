package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/jonnyom/slis/internal/cleanup"
	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/discovery"
	"github.com/jonnyom/slis/internal/notify"
	sessionmanager "github.com/jonnyom/slis/internal/session"
	"github.com/jonnyom/slis/internal/swap"
)

var rmCmd = &cobra.Command{
	Use:     "rm <slice>",
	Aliases: []string{"clean", "done"},
	Short:   "Remove a finished slice: delete its worktrees, stop its Slis session, delete merged branches",
	Long: `Remove a finished slice. For each member repo it removes the git worktree
(refusing if dirty unless --force), and by default deletes the feature branch
if it is merged (git branch -d). It also stops the slice's Slis session and
clears its grouping override, status file, and managed-slice registry entry.

Refuses if the slice is currently live (swapped in) — run 'slis deactivate' first.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		keepBranches, _ := cmd.Flags().GetBool("keep-branches")
		force, _ := cmd.Flags().GetBool("force")
		dry, _ := cmd.Flags().GetBool("dry-run")

		ws, err := config.LoadWorkspace(config.WorkspacePath())
		if err != nil {
			return fmt.Errorf("workspace not found — run `slis init` first: %w", err)
		}

		sp := config.StatePaths()
		if j, _ := swap.Load(sp.ActiveJournal); j != nil && j.Slice == name {
			return fmt.Errorf("slice %q is live (swapped in); run `slis deactivate` first", name)
		}

		sl, err := findSlice(ws, name)
		if err != nil {
			return err
		}

		opts := cleanup.Options{DeleteBranches: !keepBranches, Force: force, ActiveJournal: sp.ActiveJournal}

		if dry {
			p := cleanup.PlanRemove(sl, opts)
			fmt.Printf("Would remove slice %q:\n", p.Slice)
			for _, r := range p.Repos {
				line := fmt.Sprintf("  %s: remove worktree", r.Repo)
				if r.BranchDeleted {
					line += fmt.Sprintf(" + delete branch %s", r.Branch)
				}
				fmt.Println(line)
			}
			fmt.Println("  + stop related Slis sessions, clear grouping override + status")
			if opts.Force {
				fmt.Println("  (force: removes dirty worktrees and unmerged branches)")
			}
			return nil
		}

		manager, err := openSessionManager()
		if err != nil {
			return err
		}
		opts.KillSession = func(slice string) error {
			err := manager.Kill(cmd.Context(), slice)
			if errors.Is(err, sessionmanager.ErrGroupNotFound) {
				return nil
			}
			return err
		}

		rep, err := cleanup.Remove(ws, sl, opts)
		if err != nil {
			return err
		}
		if err := clearSliceState(sp, name, cleanupFullyRemoved(rep)); err != nil {
			return fmt.Errorf("clear slice state: %w", err)
		}

		fmt.Printf("Removed slice %q:\n", rep.Slice)
		for _, r := range rep.Repos {
			switch {
			case r.Err != "":
				fmt.Printf("  %s: FAILED — %s\n", r.Repo, r.Err)
			case r.BranchDeleted:
				fmt.Printf("  %s: worktree removed, branch %s deleted\n", r.Repo, r.Branch)
			case r.BranchKept != "":
				fmt.Printf("  %s: worktree removed, branch %s kept (%s)\n", r.Repo, r.Branch, r.BranchKept)
			default:
				fmt.Printf("  %s: worktree removed\n", r.Repo)
			}
		}
		if rep.SessionKilled {
			fmt.Println("  related Slis sessions stopped")
		}
		return nil
	},
}

// cleanupFullyRemoved reports whether every discovered member worktree was
// removed. A partial cleanup must remain registered so discovery can surface
// the missing members instead of silently forgetting unfinished work.
func cleanupFullyRemoved(rep cleanup.Report) bool {
	if len(rep.Repos) == 0 {
		return false
	}
	for _, r := range rep.Repos {
		if !r.WorktreeRemoved {
			return false
		}
	}
	return true
}

// clearSliceState removes a slice's grouping override and status. Once every
// worktree was removed successfully it also forgets the managed-slice registry
// entry, preventing an intentional clear from reappearing as "missing".
func clearSliceState(sp config.Paths, name string, forgetRegistry bool) error {
	if ov, err := discovery.LoadOverrides(sp.Overrides); err == nil {
		if _, present := ov[name]; present {
			delete(ov, name)
			_ = discovery.SaveOverrides(sp.Overrides, ov)
		}
	}
	_ = notify.RemoveStatus(sp.EventsDir, name)

	if !forgetRegistry {
		return nil
	}
	reg, exists, err := config.LoadRegistry(sp.Registry)
	if err != nil {
		return fmt.Errorf("read registry: %w", err)
	}
	if !exists {
		return nil
	}
	if _, present := reg.Slices[name]; !present {
		return nil
	}
	delete(reg.Slices, name)
	if err := config.SaveRegistry(sp.Registry, reg); err != nil {
		return fmt.Errorf("save registry: %w", err)
	}
	return nil
}

func init() {
	rmCmd.Flags().Bool("keep-branches", false, "Keep feature branches (only remove worktrees + session)")
	rmCmd.Flags().Bool("force", false, "Force-remove dirty worktrees and delete unmerged branches (-D)")
	rmCmd.Flags().Bool("dry-run", false, "Print what would be removed without changing anything")
	rootCmd.AddCommand(rmCmd)
}
