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

// plural returns "s" unless n == 1.
func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
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
	rep, err := listSlicesReport(ws, sp.Overrides, sp.ActiveJournal)
	if err != nil {
		return append(findings, doctorFinding{Level: lvlFail, Title: "could not list slices", Detail: err.Error()})
	}
	dtos := rep.Slices

	prefix := ws.Grouping.StripPrefix
	sliceIssues := 0
	for _, d := range dtos {
		var members []phantomMember
		for _, m := range d.Members {
			if !strings.HasPrefix(m.Branch, prefix+prefix) || prefix == "" {
				continue
			}
			members = append(members, phantomMember{
				repo:    m.Repo,
				primary: ws.Repos[m.Repo].Primary,
				wtPath:  m.WorktreePath,
				branch:  m.Branch,
				target:  deDoubledBranch(m.Branch, prefix),
			})
		}
		if len(members) == 0 {
			continue
		}
		sliceIssues++
		findings = append(findings, doctorFinding{
			Level:   lvlFail,
			Title:   fmt.Sprintf("doubled branch prefix: %s (%d repo%s)", d.Name, len(members), plural(len(members))),
			Detail:  "branch has strip_prefix applied twice — matches no PR, not in Graphite. --fix removes the phantom worktree(s) and adopts the real branch where possible.",
			fixDesc: "remove phantom worktrees / adopt the real branch",
			fix:     makeSliceWorktreeFixer(members),
		})
	}
	worktreeIssues := repoErrorFindings(rep.RepoErrors)
	worktreeIssues = append(worktreeIssues, skippedWorktreeFindings(rep.Skipped)...)
	worktreeIssues = append(worktreeIssues, orphanWorktreeFindings(ws)...)
	findings = append(findings, worktreeIssues...)

	switch {
	case len(dtos) == 0 && sliceIssues == 0 && len(worktreeIssues) == 0:
		findings = append(findings, doctorFinding{Level: lvlInfo, Title: "no slices discovered"})
	case sliceIssues == 0 && len(worktreeIssues) == 0:
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
// phantomMember is one doubled-prefix worktree to repair.
type phantomMember struct {
	repo, primary, wtPath, branch, target string
}

// fixDoubledWorktree repairs one phantom worktree and returns a short outcome
// phrase. It removes the phantom worktree (non-force — uncommitted work blocks
// it), then adopts the de-doubled real branch when that branch exists and is
// free; if the real branch is checked out in the primary it's left there (with
// an adopt --move hint). No branch is ever deleted.
func fixDoubledWorktree(pm phantomMember) (string, error) {
	canAdopt := pm.target != "" && branchExists(pm.primary, pm.target)
	targetInPrimary := false
	if canAdopt {
		if cur, _ := git.CurrentBranch(pm.primary); cur == pm.target {
			targetInPrimary, canAdopt = true, false
		}
	}
	if err := git.RemoveWorktree(pm.primary, pm.wtPath, false); err != nil {
		return "", fmt.Errorf("kept — worktree has local changes or is locked")
	}
	switch {
	case canAdopt:
		if _, err := git.Run(pm.primary, "worktree", "add", "--", pm.wtPath, pm.target); err != nil {
			return "removed (real branch in use elsewhere — `slis adopt --move`)", nil
		}
		return "re-pointed to the real branch", nil
	case targetInPrimary:
		return "removed (real branch is in your primary — `slis adopt --move`)", nil
	default:
		return "removed phantom worktree", nil
	}
}

// makeSliceWorktreeFixer repairs all of a slice's phantom worktrees and returns
// one concise per-repo summary. It errors only if every repo failed.
func makeSliceWorktreeFixer(members []phantomMember) func() (string, error) {
	return func() (string, error) {
		lines := make([]string, 0, len(members))
		failures := 0
		for _, pm := range members {
			outcome, err := fixDoubledWorktree(pm)
			if err != nil {
				failures++
				lines = append(lines, fmt.Sprintf("%s: %v", pm.repo, err))
				continue
			}
			lines = append(lines, fmt.Sprintf("%s: %s", pm.repo, outcome))
		}
		msg := strings.Join(lines, "; ")
		if failures == len(members) {
			return "", fmt.Errorf("%s", msg)
		}
		return msg, nil
	}
}

// branchExists reports whether refs/heads/<branch> exists in the repo at dir.
func branchExists(dir, branch string) bool {
	return git.RefExists(dir, "refs/heads/"+branch)
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
