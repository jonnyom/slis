package share

import (
	"fmt"
	"strings"

	"github.com/jonnyom/slis/internal/forge"
	"github.com/jonnyom/slis/internal/gt"
	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/internal/report"
)

type ReadStack func(string) (gt.State, error)

type PRForBranch func(string, string) (*forge.PR, error)

type BranchDiff func(string, string, string, string) (report.BranchDiffResult, error)

type repoSummary struct {
	name    string
	entries []entry
}

type entry struct {
	pr      *forge.PR
	added   int
	deleted int
}

func Markdown(slice model.Slice, readStack ReadStack, prForBranch PRForBranch, branchDiff BranchDiff) (string, error) {
	repositories := make([]repoSummary, 0, len(slice.Members))
	for _, repo := range slice.Repos() {
		member := slice.Members[repo]
		summary := repoSummary{name: repo}
		for _, branch := range stackBranches(member, readStack) {
			pr, err := prForBranch(member.WorktreePath, branch)
			if err != nil && pr == nil {
				return "", fmt.Errorf("load PR for %s/%s: %w", repo, branch, err)
			}
			if pr == nil {
				continue
			}
			diff, err := branchDiff(member.WorktreePath, repo, branch, "stat")
			if err != nil {
				return "", fmt.Errorf("diff %s/%s: %w", repo, branch, err)
			}
			if diff.Err != "" {
				return "", fmt.Errorf("diff %s/%s: %s", repo, branch, diff.Err)
			}
			if diff.Stat == nil {
				return "", fmt.Errorf("diff %s/%s returned no stats", repo, branch)
			}
			summary.entries = append(summary.entries, entry{pr: pr, added: diff.Stat.Added, deleted: diff.Stat.Deleted})
		}
		if len(summary.entries) > 0 {
			repositories = append(repositories, summary)
		}
	}

	var markdown strings.Builder
	for index, repository := range repositories {
		if len(repositories) > 1 {
			if index > 0 {
				markdown.WriteByte('\n')
			}
			fmt.Fprintf(&markdown, "**%s**\n", repository.name)
		}
		for _, summary := range repository.entries {
			fmt.Fprintf(
				&markdown,
				"[%s](%s) `+%d` `-%d`\n",
				escapeLinkText(summary.pr.Title),
				summary.pr.URL,
				summary.added,
				summary.deleted,
			)
		}
	}
	return markdown.String(), nil
}

func stackBranches(member model.SliceMember, readStack ReadStack) []string {
	state, err := readStack(member.WorktreePath)
	if err == nil {
		lineage := state.Lineage(member.Branch)
		branches := make([]string, 0, len(lineage))
		for _, branch := range lineage {
			if !branch.Trunk {
				branches = append(branches, branch.Name)
			}
		}
		if len(branches) > 0 {
			return branches
		}
	}
	return []string{member.Branch}
}

func escapeLinkText(text string) string {
	replacer := strings.NewReplacer("\\", "\\\\", "[", "\\[", "]", "\\]")
	return replacer.Replace(strings.Join(strings.Fields(text), " "))
}
