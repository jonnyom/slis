package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/discovery"
)

var unitCmd = &cobra.Command{
	Use:   "unit <unit> <slice>...",
	Short: "Collect slices into a named unit (shown together)",
	Long: `Collect one or more slices into a unit: a named collection of related slices,
such as a feature and the slices an agent spun off from it. Unlike 'slis group',
a unit never merges members — each slice keeps its own worktrees and branches,
so slices in the same repo can share a unit. A slice belongs to at most one
unit; adding it to another moves it. Reverse with 'slis ununit <slice>...'.

A slice created, adopted or imported (one path) from inside another slice —
its agent session, or a working directory inside its worktrees — joins that
slice's unit automatically.`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		unitName := strings.TrimSpace(args[0])
		if unitName == "" {
			return fmt.Errorf("unit name must not be empty")
		}
		sliceNames := args[1:]

		ws, err := config.LoadWorkspace(config.WorkspacePath())
		if err != nil {
			return fmt.Errorf("workspace not found — run `slis init` first: %w", err)
		}
		sp := config.StatePaths()
		known := make(map[string]bool)
		for _, s := range discovery.Resolve(discovery.Report(ws, sp.Registry).Slices, sp.Overrides, ws.Grouping.StripPrefix) {
			known[s.Name] = true
		}
		for _, name := range sliceNames {
			if !known[name] {
				return fmt.Errorf("slice %q not found", name)
			}
		}

		units, err := discovery.LoadUnits(sp.Overrides)
		if err != nil {
			return fmt.Errorf("read units: %w", err)
		}
		for _, name := range sliceNames {
			units[name] = unitName
		}
		if err := discovery.SaveUnits(sp.Overrides, units); err != nil {
			return fmt.Errorf("save units: %w", err)
		}
		fmt.Printf("Added %v to unit %q\n", sliceNames, unitName)
		return nil
	},
}

var ununitCmd = &cobra.Command{
	Use:   "ununit <slice>...",
	Short: "Remove slices from their unit",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sp := config.StatePaths()
		units, err := discovery.LoadUnits(sp.Overrides)
		if err != nil {
			return fmt.Errorf("read units: %w", err)
		}
		for _, name := range args {
			delete(units, name)
		}
		if err := discovery.SaveUnits(sp.Overrides, units); err != nil {
			return fmt.Errorf("save units: %w", err)
		}
		fmt.Printf("Removed %v from their unit\n", args)
		return nil
	},
}

// joinCallerUnit puts a slice made from inside another slice into that slice's
// unit, creating the unit under the parent's name when the parent has none. A
// failure is reported, never fatal: the slice itself was already made.
func joinCallerUnit(paths config.Paths, sliceName string) {
	parent := callerSlice(paths.Registry)
	if parent == "" || parent == sliceName {
		return
	}
	overridesPath := paths.Overrides
	units, err := discovery.LoadUnits(overridesPath)
	if err != nil {
		fmt.Printf("note: could not read units: %v\n", err)
		return
	}
	unitName := units[parent]
	if unitName == "" {
		unitName = parent
	}
	units[parent] = unitName
	units[sliceName] = unitName
	if err := discovery.SaveUnits(overridesPath, units); err != nil {
		fmt.Printf("note: could not save units: %v\n", err)
		return
	}
	fmt.Printf("joined unit %q (made from slice %s; undo with `slis ununit %s`)\n", unitName, parent, sliceName)
}

// callerSlice names the slice the command runs inside: the agent session's
// SLIS_SLICE, else the registered slice whose worktree holds the working
// directory (a shell tab, or an agent the user started by hand).
func callerSlice(registryPath string) string {
	if parent := strings.TrimSpace(os.Getenv("SLIS_SLICE")); parent != "" {
		return parent
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		return ""
	}
	workingDirectory = resolvePathClean(workingDirectory)
	registry, _, err := config.LoadRegistry(registryPath)
	if err != nil {
		return ""
	}
	for name, slice := range registry.Slices {
		for _, member := range slice.Members {
			worktree := resolvePathClean(member.WorktreePath)
			if workingDirectory == worktree || strings.HasPrefix(workingDirectory, worktree+string(filepath.Separator)) {
				return name
			}
		}
	}
	return ""
}

func init() {
	rootCmd.AddCommand(unitCmd)
	rootCmd.AddCommand(ununitCmd)
}
