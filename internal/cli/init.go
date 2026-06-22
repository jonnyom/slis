// Package cli implements the command-level logic for slis subcommands.
// The huh interactive picker is confined to this file; the core
// scan→build→save logic is in InitWithSelection, which has no TTY dependency.
package cli

import (
	"fmt"

	"github.com/charmbracelet/huh"
	"github.com/jonnyomahony/slis/internal/config"
)

// InitWithSelection scans root for git repos, filters to the repos named in
// selected, assembles a Workspace, saves it to config.WorkspacePath(), and
// returns the written path. It returns an error if no candidates match the
// selection or if saving fails. This function has no TTY dependency and is
// safe to call from tests and scripts.
func InitWithSelection(root string, selected []string) (string, error) {
	candidates, err := config.ScanRepos(root)
	if err != nil {
		return "", fmt.Errorf("init: scan %q: %w", root, err)
	}

	// Build a set of desired names for O(1) lookup.
	want := make(map[string]struct{}, len(selected))
	for _, name := range selected {
		want[name] = struct{}{}
	}

	var filtered []config.Candidate
	for _, c := range candidates {
		if _, ok := want[c.Name]; ok {
			filtered = append(filtered, c)
		}
	}

	if len(filtered) == 0 {
		return "", fmt.Errorf("init: no matching repos found for selection %v in %q", selected, root)
	}

	ws := config.BuildWorkspace(root, filtered)
	destPath := config.WorkspacePath()
	if err := config.SaveWorkspace(destPath, ws); err != nil {
		return "", fmt.Errorf("init: save workspace: %w", err)
	}

	return destPath, nil
}

// Init is the top-level entry point for `slis init`.
//
// If selected is non-empty, the interactive picker is skipped and
// InitWithSelection is called directly.
//
// If selected is empty, a huh multiselect form is presented so the user can
// pick which repos every slice spans. The existing workspace.yaml (if any) is
// loaded to pre-check repos already present.
func Init(root string, selected []string) (string, error) {
	if len(selected) > 0 {
		return InitWithSelection(root, selected)
	}

	return runInteractiveInit(root)
}

// runInteractiveInit presents a huh multiselect form to pick repos from root,
// then delegates to InitWithSelection.
func runInteractiveInit(root string) (string, error) {
	candidates, err := config.ScanRepos(root)
	if err != nil {
		return "", fmt.Errorf("init: scan %q: %w", root, err)
	}
	if len(candidates) == 0 {
		return "", fmt.Errorf("init: no git repos found under %q", root)
	}

	// Build the multiselect options from candidates.
	options := make([]huh.Option[string], len(candidates))
	for i, c := range candidates {
		options[i] = huh.NewOption(c.Name, c.Name)
	}

	// Pre-check repos already present in an existing workspace.yaml.
	var preSelected []string
	if existing, err := config.LoadWorkspace(config.WorkspacePath()); err == nil {
		for _, c := range candidates {
			if _, ok := existing.Repos[c.Name]; ok {
				preSelected = append(preSelected, c.Name)
			}
		}
	}

	var picked []string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Select repos every slice spans").
				Description("Space to toggle, Enter to confirm").
				Options(options...).
				Value(&picked),
		),
	)

	// Apply pre-selections if any.
	if len(preSelected) > 0 {
		picked = preSelected
	}

	if err := form.Run(); err != nil {
		return "", fmt.Errorf("init: picker: %w", err)
	}

	if len(picked) == 0 {
		return "", fmt.Errorf("init: no repos selected")
	}

	return InitWithSelection(root, picked)
}
