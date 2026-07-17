package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/discovery"
)

// resolvePathClean is defined in doctor_worktrees.go (symlink-resolving compare).

var candidatesCmd = &cobra.Command{
	Use:   "candidates",
	Short: "List worktrees slis found but has NOT ingested (opt-in import)",
	Long: `candidates lists linked worktrees that slis discovered but did not turn into
slices, because they are neither managed by slis nor registered. Import one with
` + "`slis import <path>`" + ` (or ` + "`--all`" + `), or hide it with ` + "`slis ignore`" + `.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		useJSON, _ := cmd.Flags().GetBool("json")
		ws, err := config.LoadWorkspace(config.WorkspacePath())
		if err != nil {
			return fmt.Errorf("workspace not found — run `slis init` first: %w", err)
		}
		rep := discovery.Report(ws, config.StatePaths().Registry)

		if useJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			cands := rep.Candidates
			if cands == nil {
				cands = []discovery.Candidate{}
			}
			return enc.Encode(cands)
		}

		if len(rep.Candidates) == 0 {
			fmt.Println("no new worktrees — every discovered worktree is managed or ignored")
			return nil
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "SLICE\tREPO\tBRANCH\tPATH")
		for _, c := range rep.Candidates {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", c.Slice, c.Repo, c.Branch, c.Path)
		}
		tw.Flush()
		fmt.Printf("\n%d candidate%s — `slis import <path>` or `slis import --all`\n",
			len(rep.Candidates), plural(len(rep.Candidates)))
		return nil
	},
}

var importCmd = &cobra.Command{
	Use:   "import [path]",
	Short: "Register a discovered worktree (or --all) as a managed slice",
	Long: `import records a candidate worktree in the slice registry so slis manages it
from now on (it becomes a slice and persists across discovery runs). Pass the
worktree path to import one, or --all to import every candidate. This only
updates slis's registry — it never touches git.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		all, _ := cmd.Flags().GetBool("all")
		if all == (len(args) == 1) {
			return fmt.Errorf("give a worktree path OR --all (not both, not neither)")
		}

		ws, err := config.LoadWorkspace(config.WorkspacePath())
		if err != nil {
			return fmt.Errorf("workspace not found — run `slis init` first: %w", err)
		}
		regPath := config.StatePaths().Registry
		rep := discovery.Report(ws, regPath)

		var toImport []discovery.Candidate
		if all {
			toImport = rep.Candidates
		} else {
			target := resolvePathClean(args[0])
			for _, c := range rep.Candidates {
				if resolvePathClean(c.Path) == target {
					toImport = append(toImport, c)
				}
			}
			if len(toImport) == 0 {
				return fmt.Errorf("no candidate worktree at %q — run `slis candidates` to see importable worktrees", args[0])
			}
		}
		if len(toImport) == 0 {
			fmt.Println("nothing to import — no candidate worktrees")
			return nil
		}

		reg, _, err := config.LoadRegistry(regPath)
		if err != nil {
			return fmt.Errorf("read registry: %w", err)
		}
		for _, c := range toImport {
			reg.Import(c.Slice, c.Repo, c.Branch, c.Path)
			fmt.Printf("imported %s/%s (branch %s) into slice %q\n", c.Repo, filepath.Base(c.Path), c.Branch, c.Slice)
		}
		if err := config.SaveRegistry(regPath, reg); err != nil {
			return fmt.Errorf("save registry: %w", err)
		}
		fmt.Printf("registered %d worktree%s\n", len(toImport), plural(len(toImport)))
		return nil
	},
}

var ignoreCmd = &cobra.Command{
	Use:   "ignore <path-or-glob>",
	Short: "Add a worktree path/glob to the ignore list (never ingested)",
	Long: `ignore appends a path or glob to grouping.ignore in workspace.yaml. Matching
worktrees are hidden from discovery entirely (neither slice nor candidate). Globs
support ** (any depth), * and ?. The built-in default **/.claude/worktrees/**
always applies on top of this list.

Note: saving workspace.yaml does not preserve its comments.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		pattern := args[0]
		ws, err := config.LoadWorkspace(config.WorkspacePath())
		if err != nil {
			return fmt.Errorf("workspace not found — run `slis init` first: %w", err)
		}
		for _, g := range ws.Grouping.Ignore {
			if g == pattern {
				fmt.Printf("%q is already ignored\n", pattern)
				return nil
			}
		}
		ws.Grouping.Ignore = append(ws.Grouping.Ignore, pattern)
		if err := config.SaveWorkspace(config.WorkspacePath(), ws); err != nil {
			return fmt.Errorf("save workspace: %w", err)
		}
		fmt.Printf("ignoring %q — matching worktrees will no longer be ingested\n", pattern)
		return nil
	},
}

var forgetCmd = &cobra.Command{
	Use:   "forget <slice>",
	Short: "Remove a slice from the registry (does not touch git)",
	Long: `forget removes a slice from slis's registry so slis stops managing it. It does
NOT remove worktrees or branches (use ` + "`slis rm`" + ` for that). Use it to drop a
missing slice whose worktree is gone, or to un-manage a slice.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		regPath := config.StatePaths().Registry
		reg, exists, err := config.LoadRegistry(regPath)
		if err != nil {
			return fmt.Errorf("read registry: %w", err)
		}
		if !exists {
			return fmt.Errorf("no registry yet — nothing to forget")
		}
		if _, ok := reg.Slices[name]; !ok {
			return fmt.Errorf("no registered slice %q (see `slis ls`)", name)
		}
		delete(reg.Slices, name)
		if err := config.SaveRegistry(regPath, reg); err != nil {
			return fmt.Errorf("save registry: %w", err)
		}
		fmt.Printf("forgot %q (registry only — worktrees and branches untouched)\n", name)
		return nil
	},
}

func init() {
	candidatesCmd.Flags().Bool("json", false, "Output as JSON")
	importCmd.Flags().Bool("all", false, "Import every candidate worktree")
	rootCmd.AddCommand(candidatesCmd)
	rootCmd.AddCommand(importCmd)
	rootCmd.AddCommand(ignoreCmd)
	rootCmd.AddCommand(forgetCmd)
}
