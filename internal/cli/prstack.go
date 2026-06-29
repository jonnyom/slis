package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/forge"
)

// PRStackRowDTO is a JSON-friendly representation of one repo's PR in a slice's
// stack. Number/URL/State/Title are empty when the branch has no PR.
type PRStackRowDTO struct {
	Repo           string `json:"repo"`
	Branch         string `json:"branch"`
	Number         int    `json:"number,omitempty"`
	URL            string `json:"url,omitempty"`
	State          string `json:"state,omitempty"`
	Title          string `json:"title,omitempty"`
	ReviewDecision string `json:"review_decision,omitempty"`
}

// clipboardArgv returns the clipboard tool name and args for the current OS.
// Returns (name, args, ok). On darwin: pbcopy. On linux: xclip or wl-copy.
func clipboardArgv() (string, []string, bool) {
	switch runtime.GOOS {
	case "darwin":
		return "pbcopy", nil, true
	case "linux":
		if _, err := exec.LookPath("xclip"); err == nil {
			return "xclip", []string{"-selection", "clipboard"}, true
		}
		if _, err := exec.LookPath("wl-copy"); err == nil {
			return "wl-copy", nil, true
		}
	}
	return "", nil, false
}

// writeToClipboard sends text to the system clipboard using clipboardArgv.
// Returns an error if the clipboard tool fails. If no tool is available, it
// returns (false, nil) indicating the caller should fall back to printing.
func writeToClipboard(text string) (ok bool, err error) {
	name, args, found := clipboardArgv()
	if !found {
		return false, nil
	}
	c := exec.Command(name, args...) //nolint:gosec
	c.Stdin = strings.NewReader(text)
	if runErr := c.Run(); runErr != nil {
		return false, runErr
	}
	return true, nil
}

var prStackCmd = &cobra.Command{
	Use:   "pr-stack <slice>",
	Short: "Print a shareable markdown summary of PRs for a slice",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sliceName := args[0]
		doCopy, _ := cmd.Flags().GetBool("copy")
		useJSON, _ := cmd.Flags().GetBool("json")

		ws, err := config.LoadWorkspace(config.WorkspacePath())
		if err != nil {
			return fmt.Errorf("workspace not found — run `slis init` first: %w", err)
		}

		sl, err := findSlice(ws, sliceName)
		if err != nil {
			return err
		}

		// Gather PRs in repo-sorted order.
		repos := sl.Repos()
		prs := make([]*forge.PR, 0, len(repos))
		rows := make([]PRStackRowDTO, 0, len(repos))
		for _, repo := range repos {
			m := sl.Members[repo]
			pr, _ := forge.PRForBranch(m.WorktreePath, m.Branch)
			row := PRStackRowDTO{Repo: repo, Branch: m.Branch}
			if pr != nil {
				row.Number = pr.Number
				row.URL = pr.URL
				row.State = pr.State
				row.Title = pr.Title
				row.ReviewDecision = pr.ReviewDecision
				// Prefix branch with repo name so each markdown line is clearly scoped.
				labeled := *pr
				labeled.Branch = repo + ": " + pr.Branch
				prs = append(prs, &labeled)
			} else {
				prs = append(prs, nil)
			}
			rows = append(rows, row)
		}

		if useJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(rows)
		}

		md := forge.StackMarkdown(sl.Name, prs)

		if doCopy {
			copied, copyErr := writeToClipboard(md)
			if copyErr != nil {
				// Clipboard tool found but failed — print anyway with a warning.
				fmt.Fprintln(os.Stderr, "clipboard write failed:", copyErr)
				fmt.Print(md)
				return nil
			}
			if !copied {
				fmt.Print(md)
				fmt.Fprintln(os.Stderr, "(no clipboard tool found; printed above)")
				return nil
			}
			fmt.Fprintln(os.Stderr, "copied to clipboard")
			return nil
		}

		fmt.Print(md)
		return nil
	},
}

func init() {
	prStackCmd.Flags().Bool("copy", false, "Copy the markdown to the system clipboard")
	prStackCmd.Flags().Bool("json", false, "Output the stack as JSON instead of markdown")
	rootCmd.AddCommand(prStackCmd)
}
