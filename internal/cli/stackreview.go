package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/git"
	"github.com/jonnyom/slis/internal/report"
)

// resolvePrimary validates that repo is a member of the named slice and returns
// its PRIMARY checkout path (never a worktree — the repo rule). The stack-review
// reads run there: they are pure ref-scoped reads, so any branch in the stack
// resolves regardless of which worktree has it checked out.
func resolvePrimary(ws config.Workspace, sliceName, repo string) (string, error) {
	sl, err := findSlice(ws, sliceName)
	if err != nil {
		return "", err
	}
	if _, ok := sl.Members[repo]; !ok {
		return "", fmt.Errorf("repo %q is not a member of slice %q", repo, sliceName)
	}
	rc, ok := ws.Repos[repo]
	if !ok || rc.Primary == "" {
		return "", fmt.Errorf("no primary checkout configured for repo %q", repo)
	}
	return rc.Primary, nil
}

var branchDiffCmd = &cobra.Command{
	Use:   "branch-diff <slice> <repo> <branch>",
	Short: "Diff one branch against its stack parent (or trunk)",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		sliceName, repo, branch := args[0], args[1], args[2]
		useJSON, _ := cmd.Flags().GetBool("json")

		ws, err := config.LoadWorkspace(config.WorkspacePath())
		if err != nil {
			return fmt.Errorf("workspace not found — run `slis init` first: %w", err)
		}
		primary, err := resolvePrimary(ws, sliceName, repo)
		if err != nil {
			return err
		}
		if !git.RefExists(primary, branch) {
			return fmt.Errorf("branch %q not found in repo %q", branch, repo)
		}

		res, err := report.BranchDiff(primary, repo, branch, "both")
		if err != nil {
			return err
		}

		if useJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(res)
		}

		fmt.Printf("%s › %s › vs %s\n", res.Repo, res.Branch, res.Parent)
		if res.Err != "" {
			fmt.Printf("  error: %s\n", res.Err)
			return nil
		}
		if res.Stat != nil {
			fmt.Printf("  %d files · +%d -%d\n", len(res.Stat.Files), res.Stat.Added, res.Stat.Deleted)
			for _, f := range res.Stat.Files {
				if f.Added < 0 || f.Deleted < 0 {
					fmt.Printf("    bin  %s\n", f.Path)
				} else {
					fmt.Printf("    +%-4d -%-4d %s\n", f.Added, f.Deleted, f.Path)
				}
			}
		}
		return nil
	},
}

var treeCmd = &cobra.Command{
	Use:   "tree <slice> <repo> <branch> [path]",
	Short: "List one directory level of a branch's tree",
	Args:  cobra.RangeArgs(3, 4),
	RunE: func(cmd *cobra.Command, args []string) error {
		sliceName, repo, branch := args[0], args[1], args[2]
		path := ""
		if len(args) == 4 {
			path = args[3]
		}
		useJSON, _ := cmd.Flags().GetBool("json")

		ws, err := config.LoadWorkspace(config.WorkspacePath())
		if err != nil {
			return fmt.Errorf("workspace not found — run `slis init` first: %w", err)
		}
		primary, err := resolvePrimary(ws, sliceName, repo)
		if err != nil {
			return err
		}
		if !git.RefExists(primary, branch) {
			return fmt.Errorf("branch %q not found in repo %q", branch, repo)
		}

		entries, err := git.LsTree(primary, branch, path)
		if err != nil {
			return fmt.Errorf("path %q not found in %s", path, branch)
		}
		if entries == nil {
			entries = []git.TreeEntry{}
		}

		if useJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(map[string]interface{}{
				"repo": repo, "branch": branch, "path": path, "entries": entries,
			})
		}

		for _, e := range entries {
			if e.Type == "tree" {
				fmt.Printf("  %s/\n", e.Name)
			} else {
				fmt.Printf("  %s  (%d)\n", e.Name, e.Size)
			}
		}
		return nil
	},
}

var catCmd = &cobra.Command{
	Use:   "cat <slice> <repo> <branch> <path>",
	Short: "Print a file's content at a branch's revision",
	Args:  cobra.ExactArgs(4),
	RunE: func(cmd *cobra.Command, args []string) error {
		sliceName, repo, branch, path := args[0], args[1], args[2], args[3]
		useJSON, _ := cmd.Flags().GetBool("json")

		ws, err := config.LoadWorkspace(config.WorkspacePath())
		if err != nil {
			return fmt.Errorf("workspace not found — run `slis init` first: %w", err)
		}
		primary, err := resolvePrimary(ws, sliceName, repo)
		if err != nil {
			return err
		}
		if !git.RefExists(primary, branch) {
			return fmt.Errorf("branch %q not found in repo %q", branch, repo)
		}

		if useJSON {
			// --json shares the RPC's cap/binary handling (report.FileAtRevision).
			fc, ferr := report.FileAtRevision(primary, repo, branch, path, 0)
			if ferr != nil {
				return ferr
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(fc)
		}

		// Raw mode: stream the file's exact bytes to stdout (like `git show`), no
		// cap and no stripping — the caller asked for the content verbatim.
		typ, err := git.ObjectType(primary, branch, path)
		if err != nil {
			return fmt.Errorf("path %q not found in %s", path, branch)
		}
		if typ != "blob" {
			return fmt.Errorf("path %q is a %s, not a file", path, typ)
		}
		data, err := git.ShowFile(primary, branch, path)
		if err != nil {
			return err
		}
		_, err = os.Stdout.Write(data)
		return err
	},
}

func init() {
	branchDiffCmd.Flags().Bool("json", false, "Output as JSON")
	treeCmd.Flags().Bool("json", false, "Output as JSON")
	catCmd.Flags().Bool("json", false, "Wrap the file's metadata + content as JSON")
	rootCmd.AddCommand(branchDiffCmd)
	rootCmd.AddCommand(treeCmd)
	rootCmd.AddCommand(catCmd)
}
