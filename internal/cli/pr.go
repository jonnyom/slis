package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/forge"
	"github.com/jonnyom/slis/internal/model"
)

// prRow is one resolved row for the PR table — one repo in the slice.
type prRow struct {
	repo     string
	branch   string
	pr       *forge.PR // nil when no PR exists
	overall  forge.CheckState
	pass     int
	fail     int
	pending  int
	comments int
}

// prRowsFromSlice resolves PR rows for every member of sl (sorted by repo).
// gh errors per repo are swallowed: a nil PR is used instead so the command
// never aborts on a bad remote or missing PR.
func prRowsFromSlice(sl model.Slice) []prRow {
	repos := sl.Repos() // sorted
	rows := make([]prRow, 0, len(repos))
	for _, repo := range repos {
		m := sl.Members[repo]
		pr, _ := forge.PRForBranch(m.WorktreePath, m.Branch) // nil on error or no PR
		row := prRow{
			repo:   m.Repo,
			branch: m.Branch,
			pr:     pr,
		}
		if pr != nil {
			row.overall, row.pass, row.fail, row.pending = pr.CISummary()
			row.comments = len(pr.Comments)
		}
		rows = append(rows, row)
	}
	return rows
}

// renderPRTable returns an aligned table of PR rows as a string.
// This is a pure function — testable without hitting gh.
func renderPRTable(rows []prRow) string {
	var sb stringBuilder
	tw := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "REPO\tBRANCH\tPR\tCI\tCOMMENTS\tTITLE")
	for _, r := range rows {
		prField := "-"
		ciField := "-"
		commentsField := "-"
		titleField := ""

		if r.pr != nil {
			prField = fmt.Sprintf("#%d", r.pr.Number)
			ciWord := ciStateName(r.overall)
			ciField = forge.CIEmoji(r.overall) + " " + ciWord
			commentsField = fmt.Sprintf("%d", r.comments)
			titleField = truncate(r.pr.Title, 60)
		}

		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			r.repo, r.branch, prField, ciField, commentsField, titleField)
	}
	tw.Flush()
	return sb.String()
}

// ciStateName returns the lowercase word for a CheckState.
func ciStateName(s forge.CheckState) string {
	switch s {
	case forge.CheckPass:
		return "pass"
	case forge.CheckFail:
		return "fail"
	default:
		return "pending"
	}
}

// truncate shortens s to at most n runes, appending "…" if truncated.
func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n-1]) + "…"
}

// stringBuilder wraps strings.Builder to satisfy io.Writer.
type stringBuilder struct {
	s string
}

func (b *stringBuilder) Write(p []byte) (int, error) {
	b.s += string(p)
	return len(p), nil
}

func (b *stringBuilder) String() string {
	return b.s
}

// prJSONRow is the JSON-friendly representation of a prRow.
type prJSONRow struct {
	Repo     string `json:"repo"`
	Branch   string `json:"branch"`
	Number   int    `json:"number,omitempty"`
	URL      string `json:"url,omitempty"`
	State    string `json:"state,omitempty"`
	CI       string `json:"ci,omitempty"`
	Pass     int    `json:"pass"`
	Fail     int    `json:"fail"`
	Pending  int    `json:"pending"`
	Comments int    `json:"comments"`
	Title    string `json:"title,omitempty"`
}

func prRowToJSON(r prRow) prJSONRow {
	j := prJSONRow{
		Repo:   r.repo,
		Branch: r.branch,
	}
	if r.pr != nil {
		j.Number = r.pr.Number
		j.URL = r.pr.URL
		j.State = r.pr.State
		j.CI = ciStateName(r.overall)
		j.Pass = r.pass
		j.Fail = r.fail
		j.Pending = r.pending
		j.Comments = r.comments
		j.Title = r.pr.Title
	}
	return j
}

var prCmd = &cobra.Command{
	Use:   "pr <slice>",
	Short: "Show PR status for each repo in a slice",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sliceName := args[0]
		useJSON, _ := cmd.Flags().GetBool("json")

		ws, err := config.LoadWorkspace(config.WorkspacePath())
		if err != nil {
			return fmt.Errorf("workspace not found — run `slis init` first: %w", err)
		}

		sl, err := findSlice(ws, sliceName)
		if err != nil {
			return err
		}

		rows := prRowsFromSlice(sl)

		if useJSON {
			jsonRows := make([]prJSONRow, len(rows))
			for i, r := range rows {
				jsonRows[i] = prRowToJSON(r)
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(jsonRows)
		}

		fmt.Print(renderPRTable(rows))
		return nil
	},
}

func init() {
	prCmd.Flags().Bool("json", false, "Output as JSON")
	rootCmd.AddCommand(prCmd)
}
