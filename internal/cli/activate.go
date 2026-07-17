package cli

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/internal/swap"
	"github.com/spf13/cobra"
)

// buildActivations builds the []swap.RepoActivation slice from a workspace and
// a model.Slice. It is factored out as a pure function so it can be unit-tested
// without any git side-effects.
func buildActivations(ws config.Workspace, sl model.Slice) []swap.RepoActivation {
	result := make([]swap.RepoActivation, 0, len(sl.Members))
	for _, m := range sl.Members {
		repo := ws.Repos[m.Repo]
		dr := ws.Swap.DepReconcile[m.Repo]
		result = append(result, swap.RepoActivation{
			Repo:      m.Repo,
			Primary:   repo.Primary,
			Branch:    m.Branch,
			Lockfiles: dr.Lockfiles,
		})
	}
	return result
}

var activateCmd = &cobra.Command{
	Use:   "activate <slice>",
	Short: "Activate a slice — put all repo primaries on a slis/live branch at the slice's branch tips",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sliceName := args[0]
		stash, _ := cmd.Flags().GetBool("stash")
		noReconcile, _ := cmd.Flags().GetBool("no-reconcile")

		ws, err := config.LoadWorkspace(config.WorkspacePath())
		if err != nil {
			return fmt.Errorf("workspace not found — run `slis init` first: %w", err)
		}

		sp := config.StatePaths()

		// Check whether a swap is already active.
		active, err := swap.RecoverState(sp.ActiveJournal)
		if err != nil {
			return fmt.Errorf("check active state: %w", err)
		}
		if active != nil {
			if active.Slice == sliceName {
				return fmt.Errorf("slice %q is already active; run `slis refresh` to update tips", sliceName)
			}
			return fmt.Errorf("slice %q already active; run `slis deactivate` first", active.Slice)
		}

		// Find the slice via discovery + overrides.
		dtos, err := listSlices(ws, sp.Overrides, sp.ActiveJournal)
		if err != nil {
			return fmt.Errorf("list slices: %w", err)
		}
		var found *SliceDTO
		for i := range dtos {
			if dtos[i].Name == sliceName {
				found = &dtos[i]
				break
			}
		}
		if found == nil {
			return fmt.Errorf("slice %q not found", sliceName)
		}

		// Reconstruct model.Slice from the DTO so buildActivations can work on it.
		sl := model.Slice{
			Name:    found.Name,
			Base:    found.Base,
			Members: make(map[string]model.SliceMember, len(found.Members)),
		}
		for _, m := range found.Members {
			sl.Members[m.Repo] = model.SliceMember{
				Repo:         m.Repo,
				Branch:       m.Branch,
				WorktreePath: m.WorktreePath,
				TipSHA:       m.TipSHA,
			}
		}

		repos := buildActivations(ws, sl)

		// Build primary→repoName and repoName→install lookups for the installer closure.
		primaryToRepo := make(map[string]string, len(repos))
		repoToInstall := make(map[string]string, len(repos))
		for _, ra := range repos {
			primaryToRepo[ra.Primary] = ra.Repo
			if dr, ok := ws.Swap.DepReconcile[ra.Repo]; ok {
				repoToInstall[ra.Repo] = dr.Install
			}
		}

		var installer func(primaryDir string) error
		if !noReconcile {
			installer = func(primaryDir string) error {
				repoName := primaryToRepo[primaryDir]
				install := repoToInstall[repoName]
				if install == "" {
					return nil
				}
				// SECURITY: `install` is an arbitrary shell command taken from the
				// user's own workspace.yaml (swap.dep_reconcile.<repo>.install) and
				// run via `sh -c` in the primary checkout when a lockfile changed.
				// This is by design (it's how you run `bun install` etc.), but it
				// means workspace.yaml is trusted input — slis loads it only from
				// the global XDG config dir, never from a repo, so cloning a hostile
				// repo cannot inject it. The command is printed before it runs.
				fmt.Fprintf(os.Stdout, "slis: lockfile changed in %s, running: %s\n", repoName, install)
				c := exec.Command("sh", "-c", install) //nolint:gosec // intentional: user-configured reconcile hook from global config
				c.Dir = primaryDir
				c.Stdout = os.Stdout
				c.Stderr = os.Stderr
				return c.Run()
			}
		}

		opts := swap.ActivateOptions{
			Stash:     stash,
			Installer: installer,
		}

		j, err := swap.Activate(sliceName, repos, sp.ActiveJournal, opts)

		// Print summary if we have a journal (activation succeeded, possibly with reconcile warning).
		if j != nil {
			fmt.Printf("activated slice %q:\n", sliceName)
			for _, rs := range j.Repos {
				fmt.Printf("  %s: %s at %s (tracking: %s)\n", rs.Repo, rs.TempBranch, rs.TargetSHA[:min(7, len(rs.TargetSHA))], rs.Branch)
			}
			fmt.Println("primaries are now on a slis/live branch at the slice tips (Graphite works here; do stack mutations in the worktrees).")
			fmt.Println("undo with `slis deactivate` to restore them to their prior branches.")
		}

		if err != nil {
			if j != nil {
				// Swap succeeded; reconcile failed — warn but don't error out.
				fmt.Fprintf(os.Stderr, "warning: dep-reconcile incomplete: %v\n", err)
				return nil
			}
			return err
		}

		return nil
	},
}

func init() {
	activateCmd.Flags().Bool("stash", false, "Auto-stash dirty primaries before switching")
	activateCmd.Flags().Bool("no-reconcile", false, "Skip dep-reconcile even if lockfiles changed")
	rootCmd.AddCommand(activateCmd)
}
