package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jonnyom/slis/internal/config"
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

// doctorFinding is one result line from `slis doctor`.
type doctorFinding struct {
	Level  doctorLevel `json:"level"`
	Title  string      `json:"title"`
	Detail string      `json:"detail,omitempty"`
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
		Detail: fmt.Sprintf("branch %q has strip_prefix %q applied twice — it matches no PR and isn't in Graphite. "+
			"Remove this worktree/branch and recreate the slice.", branch, prefix),
	}
}

// hookFindings checks whether the slis Claude Code hooks are installed.
func hookFindings() []doctorFinding {
	home, err := os.UserHomeDir()
	if err != nil {
		return []doctorFinding{{lvlWarn, "cannot locate home directory", err.Error()}}
	}
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	binPath, err := os.Executable()
	if err != nil {
		binPath = "slis"
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []doctorFinding{{lvlWarn, "Claude session hooks not installed",
				"no ~/.claude/settings.json — run `slis init-hooks` so notifications work"}}
		}
		return []doctorFinding{{lvlWarn, "cannot read Claude settings", err.Error()}}
	}

	var settings map[string]interface{}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &settings); err != nil {
			return []doctorFinding{{lvlWarn, "cannot parse Claude settings", settingsPath + ": " + err.Error()}}
		}
	}

	missing := hooks.MissingHooks(settings, binPath)
	if len(missing) == 0 {
		return []doctorFinding{{lvlOK, "Claude session hooks installed", ""}}
	}
	return []doctorFinding{{
		Level:  lvlWarn,
		Title:  "Claude session hooks missing: " + strings.Join(missing, ", "),
		Detail: "run `slis init-hooks` (or `slis init`) so the TUI is notified when Claude needs you",
	}}
}

// runDoctor performs all read-only checks and returns the findings.
func runDoctor() []doctorFinding {
	findings := hookFindings()

	ws, err := config.LoadWorkspace(config.WorkspacePath())
	if err != nil {
		return append(findings, doctorFinding{lvlFail, "no workspace configured",
			"workspace.yaml not found — run `slis init`"})
	}
	if f := stripPrefixFinding(ws.Grouping.StripPrefix); f != nil {
		findings = append(findings, *f)
	}

	sp := config.StatePaths()
	dtos, err := listSlices(ws, sp.Overrides, sp.ActiveJournal)
	if err != nil {
		return append(findings, doctorFinding{lvlFail, "could not list slices", err.Error()})
	}
	if len(dtos) == 0 {
		return append(findings, doctorFinding{lvlInfo, "no slices discovered", ""})
	}

	// Flag doubled-prefix branches (the "jonnyjonny/" bug) — a zero-false-positive
	// signal that a slice's branch matches no PR and isn't in Graphite. (A broad
	// "not in gt state" check was tried but is too noisy: any ad-hoc, non-Graphite
	// worktree trips it.)
	sliceIssues := 0
	for _, d := range dtos {
		for _, m := range d.Members {
			if f := doubledPrefixFinding(d.Name, m.Repo, m.Branch, ws.Grouping.StripPrefix); f != nil {
				findings = append(findings, *f)
				sliceIssues++
			}
		}
	}
	if sliceIssues == 0 {
		findings = append(findings, doctorFinding{lvlOK,
			fmt.Sprintf("%d slice(s) look healthy", len(dtos)), ""})
	}
	return findings
}

func renderDoctor(findings []doctorFinding) {
	fails, warns := 0, 0
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
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Run read-only sanity checks (hooks, branches, config) and report issues",
	Long: `doctor inspects your slis setup without changing anything:
  • are the Claude Code session hooks installed (notifications)?
  • does any slice have a doubled-prefix / Graphite-untracked branch?
  • is strip_prefix well-formed?

Use --json for machine-readable output.`,
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
		renderDoctor(findings)
		return nil
	},
}

func init() {
	doctorCmd.Flags().Bool("json", false, "Output findings as JSON")
	rootCmd.AddCommand(doctorCmd)
}
