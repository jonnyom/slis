package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/jonnyom/slis/internal/commentcache"
	"github.com/jonnyom/slis/internal/config"
	"github.com/spf13/cobra"
)

var commentsCmd = &cobra.Command{
	Use:   "comments [slice]",
	Short: "Show cached PR comments for a slice (persists after the slice is cleared)",
	Long: "Show PR comments slis has fetched, read from the on-disk cache so they\n" +
		"remain visible even after a slice (and its branch) has been removed.\n" +
		"With no argument (or --all) it shows every cached slice.",
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		useJSON, _ := cmd.Flags().GetBool("json")
		all, _ := cmd.Flags().GetBool("all")

		store, err := commentcache.Load(config.StatePaths().Comments)
		if err != nil {
			return err
		}

		// Select which slices to show.
		var want []string
		if len(args) == 1 && !all {
			if _, ok := store[args[0]]; !ok {
				return fmt.Errorf("no cached comments for slice %q", args[0])
			}
			want = []string{args[0]}
		} else {
			want = store.Slices()
		}

		if useJSON {
			out := commentcache.Store{}
			for _, s := range want {
				out[s] = store[s]
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(out)
		}

		if len(want) == 0 {
			fmt.Println("(no cached comments)")
			return nil
		}
		for _, s := range want {
			fmt.Printf("# %s\n", s)
			repos := store[s]
			for repo, rc := range repos {
				fmt.Printf("  %s #%d  %s\n", repo, rc.PR, rc.URL)
				for _, c := range rc.Comments {
					author := c.Author
					if author == "" {
						author = "?"
					}
					fmt.Printf("    %s: %s\n", author, c.Body)
				}
			}
			fmt.Println()
		}
		return nil
	},
}

func init() {
	commentsCmd.Flags().Bool("all", false, "Show every cached slice (default when no slice given)")
	commentsCmd.Flags().Bool("json", false, "Output as JSON")
	rootCmd.AddCommand(commentsCmd)
}
