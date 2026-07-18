package rpcserver

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jonnyom/slis/internal/forge"
	"github.com/jonnyom/slis/internal/notify"
	"github.com/jonnyom/slis/internal/proc"
	"github.com/jonnyom/slis/internal/report"
	"github.com/jonnyom/slis/internal/review"
	"github.com/jonnyom/slis/internal/safeterm"
	"github.com/jonnyom/slis/internal/tmuxctl"
)

// hello reports the server version, the workspace root, and the resolved session
// config the front-end needs to launch session tabs.
func (s *Server) hello() (interface{}, *rpcError) {
	specs := s.ws.Sessions.AgentList()
	agents := make([]agentResult, len(specs))
	for i, a := range specs {
		agents[i] = agentResult{Name: a.Name, Cmd: a.Cmd}
	}
	return helloResult{
		Version:       s.version,
		WorkspaceRoot: s.ws.Root,
		Sessions: sessionsResult{
			Harness:   s.ws.Sessions.HarnessName(),
			Agent:     s.ws.Sessions.AgentCommand(),
			Layout:    s.ws.Sessions.Layout,
			Autostart: s.ws.Sessions.Autostart,
			Editor:    s.ws.Sessions.Editor,
		},
		Agents: agents,
	}, nil
}

// ls returns the same payload as `slis ls --json` (stack-annotated).
func (s *Server) ls() (interface{}, *rpcError) {
	res, err := report.ListSlicesReport(s.ws, s.sp.Overrides, s.sp.ActiveJournal, true)
	if err != nil {
		return nil, serverErr(err.Error(), "")
	}
	return res, nil
}

// show returns the same payload as `slis show <slice> --json`.
func (s *Server) show(raw json.RawMessage) (interface{}, *rpcError) {
	var p sliceParams
	if rerr := decodeParams(raw, &p); rerr != nil {
		return nil, rerr
	}
	if p.Slice == "" {
		return nil, invalidParams("slice is required")
	}
	dtos, err := report.ListSlices(s.ws, s.sp.Overrides, s.sp.ActiveJournal)
	if err != nil {
		return nil, serverErr(err.Error(), "")
	}
	for i := range dtos {
		if dtos[i].Name == p.Slice {
			return report.BuildDetail(dtos[i]), nil
		}
	}
	return nil, serverErr(fmt.Sprintf("slice %q not found", p.Slice), "slice-not-found")
}

// status returns a single StatusDTO when a slice is named, else the same array
// as `slis status --json`.
func (s *Server) status(raw json.RawMessage) (interface{}, *rpcError) {
	var p optionalSliceParams
	if rerr := decodeParams(raw, &p); rerr != nil {
		return nil, rerr
	}
	if p.Slice != "" {
		return report.StatusDTO{
			Slice:  p.Slice,
			Status: notify.ReadStatus(s.sp.EventsDir, p.Slice).String(),
		}, nil
	}
	dtos, err := report.SliceStatuses(s.ws, s.sp)
	if err != nil {
		return nil, serverErr(err.Error(), "")
	}
	return dtos, nil
}

// prStack returns the same payload as `slis pr-stack <slice> --json`.
func (s *Server) prStack(raw json.RawMessage) (interface{}, *rpcError) {
	var p sliceParams
	if rerr := decodeParams(raw, &p); rerr != nil {
		return nil, rerr
	}
	if p.Slice == "" {
		return nil, invalidParams("slice is required")
	}
	sl, err := report.FindSliceIn(s.ws, s.sp, p.Slice)
	if err != nil {
		return nil, serverErr(err.Error(), "slice-not-found")
	}
	return report.PRStackRows(sl), nil
}

// comments returns the same payload as `slis comments <slice> --json`.
func (s *Server) comments(raw json.RawMessage) (interface{}, *rpcError) {
	var p sliceParams
	if rerr := decodeParams(raw, &p); rerr != nil {
		return nil, rerr
	}
	if p.Slice == "" {
		return nil, invalidParams("slice is required")
	}
	store, err := report.Comments(s.sp.Comments, p.Slice, false)
	if err != nil {
		return nil, serverErr(err.Error(), "")
	}
	return store, nil
}

// conflicts returns the same payload as `slis conflicts --json`.
func (s *Server) conflicts() (interface{}, *rpcError) {
	dto, err := report.Conflicts(s.ws, s.sp.Overrides)
	if err != nil {
		return nil, serverErr(err.Error(), "")
	}
	return dto, nil
}

// validDiffScopes and validDiffFormats bound the diff params.
var (
	validDiffScopes  = map[string]bool{"working": true, "parent": true, "trunk": true}
	validDiffFormats = map[string]bool{"stat": true, "patch": true, "both": true}
)

// diff computes a slice's diff for one scope, in the requested format.
func (s *Server) diff(raw json.RawMessage) (interface{}, *rpcError) {
	var p diffParams
	if rerr := decodeParams(raw, &p); rerr != nil {
		return nil, rerr
	}
	if p.Slice == "" {
		return nil, invalidParams("slice is required")
	}
	scope := p.Scope
	if scope == "" {
		scope = "working"
	}
	if !validDiffScopes[scope] {
		return nil, invalidParams("scope must be working|parent|trunk")
	}
	format := p.Format
	if format == "" {
		format = "both"
	}
	if !validDiffFormats[format] {
		return nil, invalidParams("format must be stat|patch|both")
	}
	sl, err := report.FindSliceIn(s.ws, s.sp, p.Slice)
	if err != nil {
		return nil, serverErr(err.Error(), "slice-not-found")
	}
	res, err := report.SliceDiffScoped(sl, scope, format)
	if err != nil {
		return nil, serverErr(err.Error(), "")
	}
	return res, nil
}

// ciLog fetches the failing-CI log excerpt for a slice's PRs (`gh run view
// --log-failed` behind forge.FailedLog). With a repo named it fetches just that
// repo; otherwise every member repo. Per-repo failures (no PR, no failing run,
// gh absent) become an Error on that repo's entry rather than failing the call.
// Read-only: it never re-runs or mutates CI.
func (s *Server) ciLog(raw json.RawMessage) (interface{}, *rpcError) {
	var p ciLogParams
	if rerr := decodeParams(raw, &p); rerr != nil {
		return nil, rerr
	}
	if p.Slice == "" {
		return nil, invalidParams("slice is required")
	}
	sl, err := report.FindSliceIn(s.ws, s.sp, p.Slice)
	if err != nil {
		return nil, serverErr(err.Error(), "slice-not-found")
	}

	repos := sl.Repos()
	if p.Repo != "" {
		if _, ok := sl.Members[p.Repo]; !ok {
			return nil, invalidParams(fmt.Sprintf("repo %q is not a member of slice %q", p.Repo, p.Slice))
		}
		repos = []string{p.Repo}
	}

	out := make([]ciLogRepoResult, 0, len(repos))
	for _, repo := range repos {
		m := sl.Members[repo]
		entry := ciLogRepoResult{Repo: repo, Branch: m.Branch}
		pr, _ := forge.PRForBranch(m.WorktreePath, m.Branch)
		if pr == nil {
			entry.Error = "no open PR for this branch"
			out = append(out, entry)
			continue
		}
		log, ferr := forge.FailedLog(m.WorktreePath, pr)
		if ferr != nil {
			entry.Error = ferr.Error()
		} else {
			entry.Log = safeterm.StripNonSGR(log)
		}
		out = append(out, entry)
	}
	return ciLogResult{Repos: out}, nil
}

// capture returns the safeterm-stripped tail of a slice's tmux session. A
// missing session or absent tmux yields empty lines rather than an error,
// mirroring the TUI's capture pane.
func (s *Server) capture(raw json.RawMessage) (interface{}, *rpcError) {
	var p captureParams
	if rerr := decodeParams(raw, &p); rerr != nil {
		return nil, rerr
	}
	if p.Slice == "" {
		return nil, invalidParams("slice is required")
	}
	text, _ := tmuxctl.CapturePane(p.Slice)
	lines := splitLines(safeterm.StripNonSGR(text))
	if p.Lines > 0 && len(lines) > p.Lines {
		lines = lines[len(lines)-p.Lines:]
	}
	return captureResult{Lines: lines}, nil
}

// procs samples the process trees behind each slice's tmux session. With no
// slice named it samples every slice; slices with no live session are omitted.
func (s *Server) procs(raw json.RawMessage) (interface{}, *rpcError) {
	var p optionalSliceParams
	if rerr := decodeParams(raw, &p); rerr != nil {
		return nil, rerr
	}

	targets := []string{p.Slice}
	if p.Slice == "" {
		dtos, err := report.ListSlices(s.ws, s.sp.Overrides, s.sp.ActiveJournal)
		if err != nil {
			return nil, serverErr(err.Error(), "")
		}
		targets = targets[:0]
		for _, d := range dtos {
			targets = append(targets, d.Name)
		}
	}

	out := make([]sliceProcsResult, 0, len(targets))
	for _, name := range targets {
		pids, err := tmuxctl.PanePIDs(name)
		if err != nil {
			continue
		}
		infos, _ := proc.SliceProcs(pids)
		procs := make([]procResult, 0, len(infos))
		total := 0.0
		for _, pi := range infos {
			procs = append(procs, procResult{PID: pi.PID, PPID: pi.PPID, Cmd: pi.Cmd, CPU: pi.CPU, Mem: pi.MemMB})
			total += pi.CPU
		}
		out = append(out, sliceProcsResult{Slice: name, Procs: procs, TotalCPU: total})
	}
	return procsResult{Slices: out}, nil
}

// reviews returns the pending inline-review comments — the same shape as `slis
// review list [slice] --json`. With a slice named it filters to that slice;
// otherwise every slice's pending comments. Strictly read-only: adding and
// sending review comments stay CLI-only (`slis review add/send`) so this sidecar
// never mutates.
func (s *Server) reviews(raw json.RawMessage) (interface{}, *rpcError) {
	var p optionalSliceParams
	if rerr := decodeParams(raw, &p); rerr != nil {
		return nil, rerr
	}
	store := review.Open(s.sp.Reviews)

	var (
		comments []review.Comment
		err      error
	)
	if p.Slice != "" {
		comments, err = store.List(p.Slice)
	} else {
		comments, err = store.ListAll()
	}
	if err != nil {
		return nil, serverErr(err.Error(), "")
	}
	if comments == nil {
		comments = []review.Comment{}
	}
	return comments, nil
}

// decodeParams unmarshals a request's params into v. Absent/null params are left
// as the zero value (each method validates its required fields).
func decodeParams(raw json.RawMessage, v interface{}) *rpcError {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return invalidParams("invalid params: " + err.Error())
	}
	return nil
}

// invalidParams builds a -32602 error.
func invalidParams(msg string) *rpcError {
	return &rpcError{Code: codeInvalidParams, Message: msg}
}

// splitLines splits captured pane text into lines, dropping a single trailing
// empty line produced by a final newline.
func splitLines(s string) []string {
	if s == "" {
		return []string{}
	}
	lines := strings.Split(s, "\n")
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	return lines
}
