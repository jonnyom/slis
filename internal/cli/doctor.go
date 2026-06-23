package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/git"
	"github.com/jonnyom/slis/internal/hooks"
)

// doctorLevel is the severity of a doctor finding.
type doctorLevel string

const (
	lvlOK   doctorLevel = "ok"
	lvlWarn doctorLevel = "warn"
	lvlFail doctorLevel = "fail"
	lvlInfo doctorLevel = "info"
)

// doctorFinding is one result line from `slis doctor`. fix (when non-nil) makes
// the finding auto-fixable: `slis doctor --fix` calls it and reports the result.
// fix/fixDesc are unexported so --json output stays data-only.
type doctorFinding struct {
	Level   doctorLevel `json:"level"`
	Title   string      `json:"title"`
	Detail  string      `json:"detail,omitempty"`
	fix     func() (string, error)
	fixDesc string
}

// stripPrefixFinding warns when a non-empty strip_prefix lacks a trailing slash
// (e.g. "jonny" makes `slis create x` produce the malformed branch "jonnyx").
func stripPrefixFinding(prefix string) *doctorFinding {
	if prefix == "" || strings.HasSuffix(prefix, "/") {
		return nil
	}
	return &doctorFinding{
		Level: lvlWarn,
		Title: fmt.Sprintf("strip_prefix %q has no trailing slash", prefix),
		Detail: fmt.Sprintf("`slis create x` would yield %q; set strip_prefix to %q in workspace.yaml.",
			prefix+"x", prefix+"/"),
	}
}

// doubledPrefixFinding flags a branch whose strip_prefix appears twice — the
// "jonnyjonny/…" bug from a fully-qualified name passed to `slis create`.
func doubledPrefixFinding(slice, repo, branch, prefix string) *doctorFinding {
	if prefix == "" || !strings.HasPrefix(branch, prefix+prefix) {
		return nil
	}
	return &doctorFinding{
		Level: lvlFail,
		Title: fmt.Sprintf("doubled branch prefix in %s/%s", slice, repo),
		Detail: fmt.Sprintf("branch %q has strip_prefix %q applied twice — it matches no PR and isn't in Graphite.",
			branch, prefix),
	}
}

// deDoubledBranch collapses a doubled strip_prefix to one (jonnyjonny/X →
// jonny/X), or returns "" when the branch isn't doubled.
func deDoubledBranch(branch, prefix string) string {
	if prefix == "" || !strings.HasPrefix(branch, prefix+prefix) {
		return ""
	}
	return branch[len(prefix):]
}

// hookFindings checks whether the slis Claude Code hooks are installed.
func hookFindings() []doctorFinding {
	home, err := os.UserHomeDir()
	if err != nil {
		return []doctorFinding{{Level: lvlWarn, Title: "cannot locate home directory", Detail: err.Error()}}
	}
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	binPath, err := os.Executable()
	if err != nil {
		binPath = "slis"
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil && !os.IsNotExist(err) {
		return []doctorFinding{{Level: lvlWarn, Title: "cannot read Claude settings", Detail: err.Error()}}
	}

	var settings map[string]interface{}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &settings); err != nil {
			return []doctorFinding{{Level: lvlWarn, Title: "cannot parse Claude settings", Detail: settingsPath + ": " + err.Error()}}
		}
	}

	missing := hooks.MissingHooks(settings, binPath)
	if len(missing) == 0 {
		return []doctorFinding{{Level: lvlOK, Title: "Claude session hooks installed"}}
	}
	return []doctorFinding{{
		Level:   lvlWarn,
		Title:   "Claude session hooks missing: " + strings.Join(missing, ", "),
		Detail:  "run `slis init-hooks` (or `slis init`) so the TUI is notified when Claude needs you",
		fixDesc: "install the missing Claude hooks",
		fix: func() (string, error) {
			changes, err := hooks.InitHooks(settingsPath, binPath)
			if err != nil {
				return "", err
			}
			if len(changes) == 0 {
				return "already installed", nil
			}
			return strings.Join(changes, ", "), nil
		},
	}}
}

// runDoctor performs all read-only checks and returns the findings (some with an
// attached fix closure).
func runDoctor() []doctorFinding {
	findings := hookFindings()

	ws, err := config.LoadWorkspace(config.WorkspacePath())
	if err != nil {
		return append(findings, doctorFinding{Level: lvlFail, Title: "no workspace configured",
			Detail: "workspace.yaml not found — run `slis init`"})
	}

	if f := stripPrefixFinding(ws.Grouping.StripPrefix); f != nil {
		prefix := ws.Grouping.StripPrefix
		wsCopy := ws
		f.fixDesc = "add a trailing slash to strip_prefix in workspace.yaml"
		f.fix = func() (string, error) {
			wsCopy.Grouping.StripPrefix = prefix + "/"
			if err := config.SaveWorkspace(config.WorkspacePath(), wsCopy); err != nil {
				return "", err
			}
			return fmt.Sprintf("strip_prefix → %q (note: any comments in workspace.yaml are not preserved)", prefix+"/"), nil
		}
		findings = append(findings, *f)
	}

	sp := config.StatePaths()
	dtos, err := listSlices(ws, sp.Overrides, sp.ActiveJournal)
	if err != nil {
		return append(findings, doctorFinding{Level: lvlFail, Title: "could not list slices", Detail: err.Error()})
	}
	if len(dtos) == 0 {
		return append(findings, doctorFinding{Level: lvlInfo, Title: "no slices discovered"})
	}

	sliceIssues := 0
	for _, d := range dtos {
		for _, m := range d.Members {
			f := doubledPrefixFinding(d.Name, m.Repo, m.Branch, ws.Grouping.StripPrefix)
			if f == nil {
				continue
			}
			sliceIssues++
			// Capture per-member values for the fixer.
			primary := ws.Repos[m.Repo].Primary
			wtPath := m.WorktreePath
			branch := m.Branch
			target := deDoubledBranch(branch, ws.Grouping.StripPrefix)

			if target != "" {
				f.Detail += fmt.Sprintf(" --fix will adopt %q if it exists, else remove the phantom worktree (branch kept).", target)
				f.fixDesc = fmt.Sprintf("adopt %q into the worktree (or prune the phantom)", target)
			} else {
				f.fixDesc = "remove the phantom worktree (branch kept)"
			}
			f.fix = makeWorktreeFixer(primary, wtPath, branch, target)
			findings = append(findings, *f)
		}
	}
	if sliceIssues == 0 {
		findings = append(findings, doctorFinding{Level: lvlOK,
			Title: fmt.Sprintf("%d slice(s) look healthy", len(dtos))})
	}
	return findings
}

// makeWorktreeFixer returns a fixer for a doubled-prefix worktree. If the
// correctly-named branch (target) exists and isn't checked out elsewhere, the
// worktree is re-pointed to it; otherwise the phantom worktree is removed. All
// git operations are non-force (uncommitted work blocks removal) and no branch
// is ever deleted, so nothing commits are lost.
func makeWorktreeFixer(primary, wtPath, branch, target string) func() (string, error) {
	return func() (string, error) {
		if target != "" && branchExists(primary, target) {
			if err := git.RemoveWorktree(primary, wtPath, false); err != nil {
				return "", fmt.Errorf("can't move worktree (uncommitted changes? locked?): %w", err)
			}
			if _, err := git.Run(primary, "worktree", "add", "--", wtPath, target); err != nil {
				return "", fmt.Errorf("removed phantom worktree but couldn't attach %q (checked out elsewhere, e.g. the primary?): %w", target, err)
			}
			return fmt.Sprintf("re-pointed worktree → %q (phantom branch %q left)", target, branch), nil
		}
		if err := git.RemoveWorktree(primary, wtPath, false); err != nil {
			return "", fmt.Errorf("can't remove phantom worktree (uncommitted changes? locked?): %w", err)
		}
		note := "no correct-named branch found"
		if target != "" {
			note = fmt.Sprintf("%q not found", target)
		}
		return fmt.Sprintf("removed phantom worktree (branch %q kept; %s)", branch, note), nil
	}
}

// branchExists reports whether refs/heads/<branch> exists in the repo at dir.
func branchExists(dir, branch string) bool {
	_, err := git.Run(dir, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
	return err == nil
}

// applyDoctorFixes runs the fix closure on every auto-fixable finding.
func applyDoctorFixes(findings []doctorFinding) {
	applied, failed, manual := 0, 0, 0
	for _, f := range findings {
		if f.fix == nil {
			if f.Level == lvlFail || f.Level == lvlWarn {
				manual++
			}
			continue
		}
		msg, err := f.fix()
		if err != nil {
			fmt.Printf("✗ %s: %v\n", f.Title, err)
			failed++
			continue
		}
		fmt.Printf("✓ %s — %s\n", f.Title, msg)
		applied++
	}
	fmt.Println()
	fmt.Printf("slis doctor --fix: %d applied, %d failed, %d need manual attention\n", applied, failed, manual)
}

func renderDoctor(findings []doctorFinding) {
	fails, warns, fixable := 0, 0, 0
	for _, f := range findings {
		var sym string
		switch f.Level {
		case lvlOK:
			sym = "✓"
		case lvlWarn:
			sym = "⚠"
			warns++
		case lvlFail:
			sym = "✗"
			fails++
		default:
			sym = "·"
		}
		if f.fix != nil {
			fixable++
		}
		fmt.Printf("%s %s\n", sym, f.Title)
		if f.Detail != "" {
			fmt.Printf("    %s\n", f.Detail)
		}
	}
	fmt.Println()
	if fails == 0 && warns == 0 {
		fmt.Println("slis doctor: all checks passed ✓")
		return
	}
	fmt.Printf("slis doctor: %d issue(s) — %d fail, %d warn\n", fails+warns, fails, warns)
	if fixable > 0 {
		fmt.Printf("run `slis doctor --fix` to fix %d of them\n", fixable)
	}
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Run read-only sanity checks (hooks, branches, config); --fix applies them",
	Long: `doctor inspects your slis setup:
  • are the Claude Code session hooks installed (notifications)?
  • does any slice have a doubled-prefix branch (the "jonnyjonny/" bug)?
  • is strip_prefix well-formed?

By default it only reports. With --fix it installs missing hooks, repairs
strip_prefix, and for a doubled-prefix worktree adopts the correctly-named
branch if it exists (else removes the phantom worktree). All git operations are
non-force and never delete a branch. Use --json for machine-readable output.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		findings := runDoctor()

		if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
			out, err := json.MarshalIndent(findings, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(out))
			return nil
		}

		if fix, _ := cmd.Flags().GetBool("fix"); fix {
			applyDoctorFixes(findings)
			return nil
		}

		renderDoctor(findings)
		return nil
	},
}

func init() {
	doctorCmd.Flags().Bool("json", false, "Output findings as JSON")
	doctorCmd.Flags().Bool("fix", false, "Apply fixes for issues that can be fixed automatically")
	rootCmd.AddCommand(doctorCmd)
}
