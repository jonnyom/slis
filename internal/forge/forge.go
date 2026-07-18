// Package forge provides a read-only wrapper around the gh CLI for retrieving
// GitHub PR information, CI status, and comments for a branch.
package forge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jonnyom/slis/internal/safeterm"
)

// ghTimeout bounds a single gh invocation. gh calls are network-bound (GitHub
// API), so a stalled connection must not hold a background concurrency slot
// open indefinitely.
const ghTimeout = 45 * time.Second

// runIDRe extracts the GitHub Actions run id from a check's detail URL, e.g.
// https://github.com/o/r/actions/runs/123456/job/789 → 123456.
var runIDRe = regexp.MustCompile(`/actions/runs/(\d+)`)

// CheckState represents the aggregated state of a single CI check.
type CheckState int

const (
	// CheckPending means the check has not yet completed (queued, in-progress, or unknown).
	CheckPending CheckState = iota
	// CheckPass means the check completed successfully (or was skipped/neutral).
	CheckPass
	// CheckFail means the check completed with a failure (failure, cancelled, timed out, or error).
	CheckFail
)

// Check represents a single CI check normalised from either a CheckRun or StatusContext.
type Check struct {
	Name  string
	State CheckState
	URL   string
}

// CommentKind distinguishes where a comment came from on GitHub: a top-level
// issue/conversation comment, a PR review submission (approval / changes
// requested / commented, with an optional body), or an inline review comment
// anchored to a diff line (e.g. Cubic).
type CommentKind int

const (
	// CommentIssue is a top-level PR conversation comment.
	CommentIssue CommentKind = iota
	// CommentReview is a PR review submission's body.
	CommentReview
	// CommentInline is a review comment anchored to a diff line.
	CommentInline
)

// Comment represents a single PR comment.
type Comment struct {
	Author    string
	Body      string
	CreatedAt string
	URL       string
	Kind      CommentKind
	// Context labels the comment: the review state ("approved",
	// "changes_requested", "commented") for a CommentReview, or the diff anchor
	// ("path:line") for a CommentInline. Empty for a CommentIssue.
	Context string
}

// PR holds the parsed representation of a GitHub pull request.
type PR struct {
	Branch         string
	Number         int
	URL            string
	State          string // OPEN / MERGED / CLOSED
	Title          string
	ReviewDecision string // APPROVED / CHANGES_REQUESTED / REVIEW_REQUIRED / ""
	Checks         []Check
	Comments       []Comment
}

// ─── internal JSON types ──────────────────────────────────────────────────────

// ghCheckEntry mirrors a single element in statusCheckRollup.  All fields are
// optional because a CheckRun and a StatusContext have different field sets.
type ghCheckEntry struct {
	Typename string `json:"__typename"`
	// CheckRun fields
	Name         string `json:"name"`
	Status       string `json:"status"`
	Conclusion   string `json:"conclusion"`
	DetailsURL   string `json:"detailsUrl"`
	WorkflowName string `json:"workflowName"`
	// StatusContext fields
	Context   string `json:"context"`
	State     string `json:"state"`
	TargetURL string `json:"targetUrl"`
}

type ghAuthor struct {
	Login string `json:"login"`
}

type ghComment struct {
	ID                string   `json:"id"`
	Author            ghAuthor `json:"author"`
	AuthorAssociation string   `json:"authorAssociation"`
	Body              string   `json:"body"`
	CreatedAt         string   `json:"createdAt"`
	URL               string   `json:"url"`
}

// ghReview mirrors one element of the `reviews` field from `gh pr view`.
type ghReview struct {
	Author      ghAuthor `json:"author"`
	Body        string   `json:"body"`
	State       string   `json:"state"` // APPROVED / CHANGES_REQUESTED / COMMENTED / DISMISSED / PENDING
	SubmittedAt string   `json:"submittedAt"`
	URL         string   `json:"url"`
}

// ghInlineComment mirrors one element of the REST
// `repos/{owner}/{repo}/pulls/{n}/comments` payload (inline review comments).
// Field names follow the REST API (snake_case), unlike `gh pr view`'s GraphQL
// camelCase.
type ghInlineComment struct {
	User         ghRESTUser `json:"user"`
	Body         string     `json:"body"`
	Path         string     `json:"path"`
	Line         int        `json:"line"`
	OriginalLine int        `json:"original_line"`
	HTMLURL      string     `json:"html_url"`
	CreatedAt    string     `json:"created_at"`
}

type ghRESTUser struct {
	Login string `json:"login"`
}

type ghPR struct {
	Number            int            `json:"number"`
	URL               string         `json:"url"`
	State             string         `json:"state"`
	Title             string         `json:"title"`
	HeadRefName       string         `json:"headRefName"`
	ReviewDecision    string         `json:"reviewDecision"`
	StatusCheckRollup []ghCheckEntry `json:"statusCheckRollup"`
	Comments          []ghComment    `json:"comments"`
	Reviews           []ghReview     `json:"reviews"`
}

// ─── normalisation helpers ────────────────────────────────────────────────────

func normalizeCheck(e ghCheckEntry) Check {
	name := e.Name
	if name == "" {
		name = e.Context
	}

	url := e.DetailsURL
	if url == "" {
		url = e.TargetURL
	}

	var state CheckState
	switch e.Typename {
	case "CheckRun":
		state = checkRunState(e.Status, e.Conclusion)
	case "StatusContext":
		state = statusContextState(e.State)
	default:
		// Unknown typename: fall back to CheckRun-style if we have status/conclusion,
		// otherwise StatusContext-style.
		if e.Status != "" || e.Conclusion != "" {
			state = checkRunState(e.Status, e.Conclusion)
		} else {
			state = statusContextState(e.State)
		}
	}

	// Name and URL are GitHub-controlled (and, for forks/external PRs,
	// attacker-controlled); strip terminal escapes before they reach a render
	// path or an LLM prompt.
	return Check{Name: safeterm.Strip(name), State: state, URL: safeterm.Strip(url)}
}

// checkRunState maps a CheckRun's (status, conclusion) pair to a CheckState.
//
//   - conclusion == "SUCCESS"                        → Pass
//   - conclusion ∈ {FAILURE, CANCELLED, TIMED_OUT, ERROR} → Fail
//   - conclusion ∈ {SKIPPED, NEUTRAL}               → Pass (non-blocking)
//   - status != "COMPLETED" or conclusion == ""      → Pending
func checkRunState(status, conclusion string) CheckState {
	switch strings.ToUpper(conclusion) {
	case "SUCCESS", "SKIPPED", "NEUTRAL":
		return CheckPass
	case "FAILURE", "CANCELLED", "TIMED_OUT", "ERROR":
		return CheckFail
	}
	// conclusion is empty or unrecognised; treat as pending unless completed
	if strings.ToUpper(status) == "COMPLETED" {
		// Completed but unknown conclusion — treat as pending to be safe.
		return CheckPending
	}
	return CheckPending
}

// statusContextState maps a legacy StatusContext state string to a CheckState.
func statusContextState(state string) CheckState {
	switch strings.ToUpper(state) {
	case "SUCCESS":
		return CheckPass
	case "FAILURE", "ERROR":
		return CheckFail
	}
	return CheckPending
}

// ─── Public API ───────────────────────────────────────────────────────────────

// reviewIsComment reports whether a PR review should surface as a comment line.
// PENDING/DISMISSED reviews are never shown; otherwise a review is shown only
// when it carries body text — a bare approval is conveyed by the review-decision
// badge, not a comment.
func reviewIsComment(state, body string) bool {
	switch strings.ToUpper(state) {
	case "PENDING", "DISMISSED":
		return false
	}
	return strings.TrimSpace(body) != ""
}

// ParsePR parses the JSON payload produced by
//
//	gh pr view <branch> --json number,url,state,title,headRefName,statusCheckRollup,reviewDecision,comments,reviews
//
// and returns a *PR populated with normalised Checks and Comments (issue
// comments plus bodied review submissions). Inline review comments are fetched
// separately (see PRForBranch).
// branch is stored verbatim on the returned PR (gh puts headRefName in the JSON,
// but the caller may supply a different local branch name).
func ParsePR(branch string, data []byte) (*PR, error) {
	var raw ghPR
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("forge: unmarshal PR JSON: %w", err)
	}

	checks := make([]Check, 0, len(raw.StatusCheckRollup))
	for _, e := range raw.StatusCheckRollup {
		checks = append(checks, normalizeCheck(e))
	}

	comments := make([]Comment, 0, len(raw.Comments)+len(raw.Reviews))
	for _, c := range raw.Comments {
		comments = append(comments, Comment{
			Author:    safeterm.Strip(c.Author.Login),
			Body:      safeterm.Strip(c.Body),
			CreatedAt: safeterm.Strip(c.CreatedAt),
			URL:       safeterm.Strip(c.URL),
			Kind:      CommentIssue,
		})
	}
	for _, r := range raw.Reviews {
		if !reviewIsComment(r.State, r.Body) {
			continue
		}
		comments = append(comments, Comment{
			Author:    safeterm.Strip(r.Author.Login),
			Body:      safeterm.Strip(r.Body),
			CreatedAt: safeterm.Strip(r.SubmittedAt),
			URL:       safeterm.Strip(r.URL),
			Kind:      CommentReview,
			Context:   strings.ToLower(safeterm.Strip(r.State)),
		})
	}

	// Every string below is sourced from `gh` (i.e. GitHub) and is rendered in
	// the TUI / shareable markdown; sanitise terminal escapes at this boundary.
	return &PR{
		Branch:         branch,
		Number:         raw.Number,
		URL:            safeterm.Strip(raw.URL),
		State:          safeterm.Strip(raw.State),
		Title:          safeterm.Strip(raw.Title),
		ReviewDecision: safeterm.Strip(raw.ReviewDecision),
		Checks:         checks,
		Comments:       comments,
	}, nil
}

// jsonFields is the fixed set of fields we request from gh.
const jsonFields = "number,url,state,title,headRefName,statusCheckRollup,reviewDecision,comments,reviews"

// PRForBranch runs `gh pr view <branch> --json ...` in repoDir and returns the
// parsed PR.
//
// Returns (nil, nil) when:
//   - gh is not installed / not on PATH (degrade gracefully)
//   - there is no open PR for the branch (gh exits non-zero with a recognisable message)
//
// Returns (nil, err) for any other gh failure.
func PRForBranch(repoDir, branch string) (*PR, error) {
	if !Available() {
		return nil, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), ghTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", "pr", "view", branch, "--json", jsonFields)
	cmd.Dir = repoDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		msg := strings.ToLower(stderr.String())
		if strings.Contains(msg, "no pull requests found") ||
			strings.Contains(msg, "no open pull requests") {
			return nil, nil
		}
		return nil, fmt.Errorf("forge: gh pr view: %w\nstderr: %s", err, stderr.String())
	}

	pr, err := ParsePR(branch, stdout.Bytes())
	if err != nil || pr == nil {
		return pr, err
	}

	// Inline review comments (e.g. Cubic) are not exposed by `gh pr view`; fetch
	// them via the REST API and merge. A failure here is non-fatal — the PR still
	// renders with issue + review comments — so we return the populated PR
	// alongside the (wrapped) error; callers tolerate per-repo degradation.
	inline, ierr := inlineComments(repoDir, pr.URL, pr.Number)
	pr.Comments = append(pr.Comments, inline...)
	sortCommentsByTime(pr.Comments)
	if ierr != nil {
		return pr, fmt.Errorf("forge: inline comments: %w", ierr)
	}
	return pr, nil
}

// prURLRe extracts owner/repo from a PR URL, e.g.
// https://github.com/Noryai/nory/pull/8107 → ("Noryai", "nory").
var prURLRe = regexp.MustCompile(`github\.com/([^/]+)/([^/]+)/pull/\d+`)

// parsePRURL extracts the owner and repo from a GitHub PR URL.
func parsePRURL(prURL string) (owner, repo string, ok bool) {
	m := prURLRe.FindStringSubmatch(prURL)
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}

// inlineComments fetches a PR's inline review comments via
// `gh api repos/{owner}/{repo}/pulls/{number}/comments --paginate`, run in
// repoDir. Returns (nil, nil) when gh is absent or the URL is unparseable.
func inlineComments(repoDir, prURL string, number int) ([]Comment, error) {
	if !Available() {
		return nil, nil
	}
	owner, repo, ok := parsePRURL(prURL)
	if !ok {
		return nil, nil
	}
	path := fmt.Sprintf("repos/%s/%s/pulls/%d/comments", owner, repo, number)
	ctx, cancel := context.WithTimeout(context.Background(), ghTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", "api", path, "--paginate")
	cmd.Dir = repoDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("gh api %s: %w\nstderr: %s", path, err, stderr.String())
	}
	return ParseInlineComments(stdout.Bytes())
}

// ParseInlineComments parses the REST `pulls/{n}/comments` JSON array into
// Comments tagged CommentInline with a "path:line" Context. Pure and testable.
func ParseInlineComments(data []byte) ([]Comment, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, nil
	}
	var raw []ghInlineComment
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("forge: unmarshal inline comments: %w", err)
	}
	out := make([]Comment, 0, len(raw))
	for _, c := range raw {
		line := c.Line
		if line == 0 {
			line = c.OriginalLine
		}
		ctx := safeterm.Strip(c.Path)
		if line > 0 {
			ctx = fmt.Sprintf("%s:%d", ctx, line)
		}
		out = append(out, Comment{
			Author:    safeterm.Strip(c.User.Login),
			Body:      safeterm.Strip(c.Body),
			CreatedAt: safeterm.Strip(c.CreatedAt),
			URL:       safeterm.Strip(c.HTMLURL),
			Kind:      CommentInline,
			Context:   ctx,
		})
	}
	return out, nil
}

// sortCommentsByTime stably orders comments by their CreatedAt timestamp.
// GitHub timestamps are RFC3339, which sorts correctly lexically; empty
// timestamps sort first.
func sortCommentsByTime(cs []Comment) {
	sort.SliceStable(cs, func(i, j int) bool { return cs[i].CreatedAt < cs[j].CreatedAt })
}

// CISummary aggregates the PR's checks and returns an overall state plus counts.
//
//   - If there are no checks: overall is CheckPending, all counts are 0.
//   - If any check failed:    overall is CheckFail.
//   - Else if any is pending: overall is CheckPending.
//   - Else:                   overall is CheckPass (and pass > 0).
func (p *PR) CISummary() (overall CheckState, pass, fail, pending int) {
	for _, c := range p.Checks {
		switch c.State {
		case CheckPass:
			pass++
		case CheckFail:
			fail++
		case CheckPending:
			pending++
		}
	}

	switch {
	case fail > 0:
		overall = CheckFail
	case pending > 0:
		overall = CheckPending
	case pass > 0:
		overall = CheckPass
	default:
		// No checks at all.
		overall = CheckPending
	}
	return
}

// FailingChecks returns all checks whose State is CheckFail.
func (p *PR) FailingChecks() []Check {
	var out []Check
	for _, c := range p.Checks {
		if c.State == CheckFail {
			out = append(out, c)
		}
	}
	return out
}

// failingRunIDs returns the unique GitHub Actions run ids backing pr's failing
// checks, parsed from their detail URLs (in check order).
func failingRunIDs(pr *PR) []string {
	seen := map[string]bool{}
	var ids []string
	for _, c := range pr.FailingChecks() {
		if m := runIDRe.FindStringSubmatch(c.URL); m != nil && !seen[m[1]] {
			seen[m[1]] = true
			ids = append(ids, m[1])
		}
	}
	return ids
}

// RerunFailedChecks re-triggers the failed jobs of the Actions runs behind pr's
// failing checks (`gh run rerun <id> --failed`, run in repoDir). Returns how many
// runs were re-triggered. Mutating — the one write slis makes to CI.
func RerunFailedChecks(repoDir string, pr *PR) (int, error) {
	if !Available() {
		return 0, fmt.Errorf("gh not found on PATH")
	}
	ids := failingRunIDs(pr)
	if len(ids) == 0 {
		return 0, nil
	}
	n := 0
	var firstErr error
	for _, id := range ids {
		cmd := exec.Command("gh", "run", "rerun", id, "--failed")
		cmd.Dir = repoDir
		if out, err := cmd.CombinedOutput(); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("gh run rerun %s: %w: %s", id, err, strings.TrimSpace(string(out)))
			}
			continue
		}
		n++
	}
	return n, firstErr
}

// FailedLog returns the failed-step logs for pr's first failing check's run
// (`gh run view <id> --log-failed`, run in repoDir), for display inside slis.
func FailedLog(repoDir string, pr *PR) (string, error) {
	if !Available() {
		return "", fmt.Errorf("gh not found on PATH")
	}
	ids := failingRunIDs(pr)
	if len(ids) == 0 {
		return "", fmt.Errorf("no failing CI run found")
	}
	cmd := exec.Command("gh", "run", "view", ids[0], "--log-failed")
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("gh run view --log-failed: %w", err)
	}
	return string(out), nil
}

// Available reports whether the gh binary is on PATH.
func Available() bool {
	_, err := exec.LookPath("gh")
	return err == nil
}

// CIStateName returns the lowercase rollup word for a CheckState:
// Pass → "pass", Fail → "fail", Pending → "pending".
func CIStateName(s CheckState) string {
	switch s {
	case CheckPass:
		return "pass"
	case CheckFail:
		return "fail"
	default:
		return "pending"
	}
}

// CIEmoji returns the display emoji for a given CheckState.
// Pass → ✅, Fail → ❌, Pending → ⏳.
func CIEmoji(s CheckState) string {
	switch s {
	case CheckPass:
		return "✅"
	case CheckFail:
		return "❌"
	default:
		return "⏳"
	}
}

// StackMarkdown renders a shareable markdown summary of a set of PRs (a stack /
// slice). prs are rendered in order; nil entries are skipped. CI emoji: Pass=✅
// Fail=❌ Pending=⏳. A comment count is shown only when >0.
func StackMarkdown(title string, prs []*PR) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "### Stack: %s\n\n", title)
	for _, pr := range prs {
		if pr == nil {
			continue
		}
		overall, _, _, _ := pr.CISummary()
		ciEmoji := CIEmoji(overall)
		// Only show CI emoji if there are checks; otherwise use ⏳ (no-checks pending).
		line := fmt.Sprintf("- **%s** — [#%d](%s) %s", pr.Branch, pr.Number, pr.URL, ciEmoji)
		if len(pr.Comments) > 0 {
			line += fmt.Sprintf(" · 💬 %d", len(pr.Comments))
		}
		line += fmt.Sprintf(" — %s", pr.Title)
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	return sb.String()
}
