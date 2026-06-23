// Package forge provides a read-only wrapper around the gh CLI for retrieving
// GitHub PR information, CI status, and comments for a branch.
package forge

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

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

// Comment represents a single PR comment.
type Comment struct {
	Author    string
	Body      string
	CreatedAt string
	URL       string
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

type ghPR struct {
	Number            int            `json:"number"`
	URL               string         `json:"url"`
	State             string         `json:"state"`
	Title             string         `json:"title"`
	HeadRefName       string         `json:"headRefName"`
	ReviewDecision    string         `json:"reviewDecision"`
	StatusCheckRollup []ghCheckEntry `json:"statusCheckRollup"`
	Comments          []ghComment    `json:"comments"`
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

	return Check{Name: name, State: state, URL: url}
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

// ParsePR parses the JSON payload produced by
//
//	gh pr view <branch> --json number,url,state,title,headRefName,statusCheckRollup,reviewDecision,comments
//
// and returns a *PR populated with normalised Checks and Comments.
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

	comments := make([]Comment, 0, len(raw.Comments))
	for _, c := range raw.Comments {
		comments = append(comments, Comment{
			Author:    c.Author.Login,
			Body:      c.Body,
			CreatedAt: c.CreatedAt,
			URL:       c.URL,
		})
	}

	return &PR{
		Branch:         branch,
		Number:         raw.Number,
		URL:            raw.URL,
		State:          raw.State,
		Title:          raw.Title,
		ReviewDecision: raw.ReviewDecision,
		Checks:         checks,
		Comments:       comments,
	}, nil
}

// jsonFields is the fixed set of fields we request from gh.
const jsonFields = "number,url,state,title,headRefName,statusCheckRollup,reviewDecision,comments"

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

	cmd := exec.Command("gh", "pr", "view", branch, "--json", jsonFields)
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

	return ParsePR(branch, stdout.Bytes())
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

// Available reports whether the gh binary is on PATH.
func Available() bool {
	_, err := exec.LookPath("gh")
	return err == nil
}
