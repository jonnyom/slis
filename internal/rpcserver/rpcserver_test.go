package rpcserver

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/internal/notify"
	"github.com/jonnyom/slis/internal/report"
	"github.com/jonnyom/slis/internal/review"
	"github.com/jonnyom/slis/testutil"
)

// harness wires a Server to two in-process pipes and runs Serve in a goroutine.
// send writes one request line; recv reads one response/notification line.
type harness struct {
	t      *testing.T
	sp     config.Paths
	toSrv  *io.PipeWriter
	dec    *bufio.Reader
	cancel context.CancelFunc
	done   chan struct{}
}

func newHarness(t *testing.T, ws config.Workspace) *harness {
	t.Helper()

	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)
	sp := config.StatePaths()
	if err := sp.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs: %v", err)
	}

	srv := New(ws, sp, "test-version")

	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = srv.Serve(ctx, inR, outW)
		_ = outW.Close()
		close(done)
	}()

	h := &harness{t: t, sp: sp, toSrv: inW, dec: bufio.NewReader(outR), cancel: cancel, done: done}
	t.Cleanup(func() {
		cancel()
		_ = inW.Close()
		// Wait for Serve (and its watcher/handler goroutines) to fully stop before
		// t.TempDir removes the state and repo dirs, so nothing races the cleanup.
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("server did not shut down within 5s")
		}
	})
	return h
}

func (h *harness) send(line string) {
	h.t.Helper()
	if _, err := io.WriteString(h.toSrv, line+"\n"); err != nil {
		h.t.Fatalf("send: %v", err)
	}
}

// recv reads one JSON line from the server.
func (h *harness) recv() response {
	h.t.Helper()
	line, err := h.dec.ReadBytes('\n')
	if err != nil {
		h.t.Fatalf("recv: %v", err)
	}
	var resp response
	if err := json.Unmarshal(line, &resp); err != nil {
		h.t.Fatalf("recv unmarshal %q: %v", line, err)
	}
	return resp
}

// call sends a request and returns the matching response. Any sessionEvent
// notifications that arrive first are skipped.
func (h *harness) call(id int, method, params string) response {
	h.t.Helper()
	if params == "" {
		h.send(`{"jsonrpc":"2.0","id":` + strconv.Itoa(id) + `,"method":"` + method + `"}`)
	} else {
		h.send(`{"jsonrpc":"2.0","id":` + strconv.Itoa(id) + `,"method":"` + method + `","params":` + params + `}`)
	}
	for {
		resp := h.recv()
		if len(resp.ID) == 0 || string(resp.ID) == "null" {
			continue // a notification; keep reading for our reply
		}
		return resp
	}
}

// makeWorkspace builds a 3-repo workspace: web+api on jonny/checkout (one
// slice), ops on jonny/other (a second slice), stripping the jonny/ prefix.
func makeWorkspace(t *testing.T) config.Workspace {
	t.Helper()
	web := testutil.NewRepo(t)
	api := testutil.NewRepo(t)
	ops := testutil.NewRepo(t)

	base := t.TempDir()
	testutil.AddWorktree(t, web, "jonny/checkout", filepath.Join(base, "web-checkout"))
	testutil.AddWorktree(t, api, "jonny/checkout", filepath.Join(base, "api-checkout"))
	testutil.AddWorktree(t, ops, "jonny/other", filepath.Join(base, "ops-other"))

	return config.Workspace{
		Root: base,
		Repos: map[string]config.Repo{
			"web": {Primary: web, DefaultBranch: "main"},
			"api": {Primary: api, DefaultBranch: "main"},
			"ops": {Primary: ops, DefaultBranch: "main"},
		},
		Grouping: config.Grouping{Strategy: "branch-name", StripPrefix: "jonny/"},
	}
}

func decodeResult(t *testing.T, resp response, v interface{}) {
	t.Helper()
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	b, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
}

func TestHello(t *testing.T) {
	ws := makeWorkspace(t)
	h := newHarness(t, ws)

	resp := h.call(1, "hello", "")
	var hr helloResult
	decodeResult(t, resp, &hr)
	if hr.Version != "test-version" {
		t.Errorf("version = %q, want test-version", hr.Version)
	}
	if hr.WorkspaceRoot != ws.Root {
		t.Errorf("workspaceRoot = %q, want %q", hr.WorkspaceRoot, ws.Root)
	}
	// Unconfigured sessions resolve to the Go-TUI defaults.
	if hr.Sessions.Harness != "claude" || hr.Sessions.Agent != "claude" {
		t.Errorf("default sessions = %+v, want harness/agent claude", hr.Sessions)
	}
	if hr.Sessions.Layout != "" || hr.Sessions.Autostart {
		t.Errorf("default sessions = %+v, want empty layout and autostart false", hr.Sessions)
	}
	// Unconfigured agents resolve to a single default derived from harness/agent.
	if len(hr.Agents) != 1 {
		t.Fatalf("agents = %+v, want 1 default agent", hr.Agents)
	}
	if hr.Agents[0].Name != "claude" || len(hr.Agents[0].Cmd) != 1 || hr.Agents[0].Cmd[0] != "claude" {
		t.Errorf("default agent = %+v, want claude/[claude]", hr.Agents[0])
	}
}

func TestHelloExposesConfiguredAgents(t *testing.T) {
	ws := makeWorkspace(t)
	ws.Sessions = config.Sessions{Agents: []config.AgentSpec{
		{Name: "claude", Cmd: []string{"claude"}},
		{Name: "codex", Cmd: []string{"codex", "--full-auto"}},
	}}
	h := newHarness(t, ws)

	resp := h.call(1, "hello", "")
	var hr helloResult
	decodeResult(t, resp, &hr)

	if len(hr.Agents) != 2 {
		t.Fatalf("agents = %+v, want 2", hr.Agents)
	}
	if hr.Agents[1].Name != "codex" || len(hr.Agents[1].Cmd) != 2 || hr.Agents[1].Cmd[1] != "--full-auto" {
		t.Errorf("second agent = %+v, want codex/[codex --full-auto]", hr.Agents[1])
	}
}

func TestHelloResolvesSessionsConfig(t *testing.T) {
	ws := makeWorkspace(t)
	ws.Sessions = config.Sessions{Harness: "codex", Layout: "repos", Autostart: true, DefaultAgent: "Codex"}
	h := newHarness(t, ws)

	resp := h.call(1, "hello", "")
	var hr helloResult
	decodeResult(t, resp, &hr)

	if hr.Sessions.Harness != "codex" {
		t.Errorf("harness = %q, want codex", hr.Sessions.Harness)
	}
	if hr.Sessions.Agent != "codex" {
		t.Errorf("agent = %q, want codex (resolved from harness)", hr.Sessions.Agent)
	}
	if hr.Sessions.Layout != "repos" {
		t.Errorf("layout = %q, want repos", hr.Sessions.Layout)
	}
	if !hr.Sessions.Autostart {
		t.Errorf("autostart = false, want true")
	}
	if hr.Sessions.DefaultAgent != "Codex" {
		t.Errorf("default agent = %q, want Codex", hr.Sessions.DefaultAgent)
	}
}

func TestLsMatchesReportBuilder(t *testing.T) {
	ws := makeWorkspace(t)
	h := newHarness(t, ws)

	resp := h.call(1, "ls", "")
	var got report.LsResultDTO
	decodeResult(t, resp, &got)

	want, err := report.ListSlicesReport(ws, h.sp.Overrides, h.sp.ActiveJournal, true)
	if err != nil {
		t.Fatalf("ListSlicesReport: %v", err)
	}
	if len(got.Slices) != len(want.Slices) {
		t.Fatalf("ls returned %d slices, want %d", len(got.Slices), len(want.Slices))
	}
	names := map[string]bool{}
	for _, s := range got.Slices {
		names[s.Name] = true
	}
	if !names["checkout"] || !names["other"] {
		t.Errorf("ls slices = %v, want checkout+other", names)
	}
}

func TestShow(t *testing.T) {
	ws := makeWorkspace(t)
	h := newHarness(t, ws)

	resp := h.call(1, "show", `{"slice":"checkout"}`)
	var got report.SliceDetailDTO
	decodeResult(t, resp, &got)
	if got.Name != "checkout" {
		t.Errorf("show name = %q, want checkout", got.Name)
	}
	if len(got.Members) != 2 {
		t.Errorf("show members = %d, want 2 (web+api)", len(got.Members))
	}
}

func TestShowNotFound(t *testing.T) {
	h := newHarness(t, makeWorkspace(t))
	resp := h.call(1, "show", `{"slice":"nope"}`)
	if resp.Error == nil {
		t.Fatal("expected error for missing slice")
	}
	if resp.Error.Data == nil || resp.Error.Data.Kind != "slice-not-found" {
		t.Errorf("error kind = %+v, want slice-not-found", resp.Error.Data)
	}
}

func TestStatusAllAndSingle(t *testing.T) {
	ws := makeWorkspace(t)
	h := newHarness(t, ws)

	if err := notify.WriteStatus(h.sp.EventsDir, "checkout", model.SessWaitingInput, time.Now().UnixNano()); err != nil {
		t.Fatalf("WriteStatus: %v", err)
	}

	// Single slice → one object.
	respOne := h.call(1, "status", `{"slice":"checkout"}`)
	var one report.StatusDTO
	decodeResult(t, respOne, &one)
	if one.Status != "waiting-input" {
		t.Errorf("single status = %q, want waiting-input", one.Status)
	}

	// No slice → array of every slice.
	respAll := h.call(2, "status", "")
	var all []report.StatusDTO
	decodeResult(t, respAll, &all)
	if len(all) != 2 {
		t.Fatalf("status all = %d entries, want 2", len(all))
	}
}

func TestDiffWorkingScope(t *testing.T) {
	ws := makeWorkspace(t)
	h := newHarness(t, ws)

	// Dirty the web worktree so the working-tree diff is non-empty.
	wt := filepath.Join(ws.Root, "web-checkout")
	writeFile(t, filepath.Join(wt, "new.txt"), "hello\nworld\n")

	resp := h.call(1, "diff", `{"slice":"checkout","scope":"working","format":"both"}`)
	var got report.DiffResult
	decodeResult(t, resp, &got)

	var webPatch string
	for _, r := range got.Repos {
		if r.Repo == "web" {
			if r.Patch == nil {
				t.Fatal("web patch missing in both-format diff")
			}
			webPatch = *r.Patch
		}
	}
	if webPatch == "" {
		t.Error("expected a non-empty web working-tree diff after dirtying it")
	}
}

func TestDiffInvalidScope(t *testing.T) {
	h := newHarness(t, makeWorkspace(t))
	resp := h.call(1, "diff", `{"slice":"checkout","scope":"bogus"}`)
	if resp.Error == nil || resp.Error.Code != codeInvalidParams {
		t.Fatalf("expected invalid-params error, got %+v", resp.Error)
	}
}

func TestCILogRequiresSlice(t *testing.T) {
	h := newHarness(t, makeWorkspace(t))
	resp := h.call(1, "ciLog", `{}`)
	if resp.Error == nil || resp.Error.Code != codeInvalidParams {
		t.Fatalf("expected invalid-params for missing slice, got %+v", resp.Error)
	}
}

func TestCILogSliceNotFound(t *testing.T) {
	h := newHarness(t, makeWorkspace(t))
	resp := h.call(1, "ciLog", `{"slice":"nope"}`)
	if resp.Error == nil || resp.Error.Data == nil || resp.Error.Data.Kind != "slice-not-found" {
		t.Fatalf("expected slice-not-found error, got %+v", resp.Error)
	}
}

func TestCILogUnknownRepoRejected(t *testing.T) {
	h := newHarness(t, makeWorkspace(t))
	resp := h.call(1, "ciLog", `{"slice":"checkout","repo":"ops"}`)
	if resp.Error == nil || resp.Error.Code != codeInvalidParams {
		t.Fatalf("expected invalid-params for non-member repo, got %+v", resp.Error)
	}
}

// TestCILogNoPR: with no gh (or no PR), each member repo comes back with an
// Error rather than a Log, and the call itself succeeds. Asserts the read-only,
// degrade-gracefully contract; independent of whether gh is installed since the
// throwaway repos have no real PRs.
func TestCILogNoPR(t *testing.T) {
	h := newHarness(t, makeWorkspace(t))
	resp := h.call(1, "ciLog", `{"slice":"checkout"}`)
	var got ciLogResult
	decodeResult(t, resp, &got)
	if len(got.Repos) != 2 {
		t.Fatalf("ciLog repos = %d, want 2 (web+api)", len(got.Repos))
	}
	for _, r := range got.Repos {
		if r.Log != "" {
			t.Errorf("repo %s unexpectedly has a log without a PR: %q", r.Repo, r.Log)
		}
		if r.Error == "" {
			t.Errorf("repo %s should carry an error when it has no PR", r.Repo)
		}
	}
}

func TestConflicts(t *testing.T) {
	h := newHarness(t, makeWorkspace(t))
	resp := h.call(1, "conflicts", "")
	var got report.ConflictsDTO
	decodeResult(t, resp, &got)
	if got.Overlaps == nil || got.Incomplete == nil {
		t.Errorf("conflicts arrays should be non-nil: %+v", got)
	}
}

func TestUnknownMethod(t *testing.T) {
	h := newHarness(t, makeWorkspace(t))
	resp := h.call(1, "nope", "")
	if resp.Error == nil || resp.Error.Code != codeMethodNotFound {
		t.Fatalf("expected method-not-found, got %+v", resp.Error)
	}
}

func TestParseError(t *testing.T) {
	h := newHarness(t, makeWorkspace(t))
	h.send(`{not json`)
	resp := h.recv()
	if resp.Error == nil || resp.Error.Code != codeParse {
		t.Fatalf("expected parse error, got %+v", resp.Error)
	}
	if string(resp.ID) != "null" {
		t.Errorf("parse-error id = %q, want null", resp.ID)
	}
}

func TestNotificationGetsNoReply(t *testing.T) {
	h := newHarness(t, makeWorkspace(t))
	// A request with no id is a notification: it must not be replied to. Send it,
	// then a real call; the only reply we should read is the real call's.
	h.send(`{"jsonrpc":"2.0","method":"hello"}`)
	resp := h.call(7, "hello", "")
	if string(resp.ID) != "7" {
		t.Errorf("reply id = %q, want 7 (notification must get no reply)", resp.ID)
	}
}

func TestSessionEventNotification(t *testing.T) {
	ws := makeWorkspace(t)
	h := newHarness(t, ws)

	// Give the watcher goroutine a moment to take its initial snapshot.
	time.Sleep(50 * time.Millisecond)
	if err := notify.WriteStatus(h.sp.EventsDir, "checkout", model.SessWaitingInput, time.Now().UnixNano()); err != nil {
		t.Fatalf("WriteStatus: %v", err)
	}

	// The next line from the server should be a sessionEvent notification.
	deadline := time.After(3 * time.Second)
	type read struct {
		resp response
		err  error
	}
	ch := make(chan read, 1)
	go func() {
		line, err := h.dec.ReadBytes('\n')
		if err != nil {
			ch <- read{err: err}
			return
		}
		var r response
		_ = json.Unmarshal(line, &r)
		ch <- read{resp: r}
	}()

	select {
	case <-deadline:
		t.Fatal("no sessionEvent notification arrived")
	case got := <-ch:
		if got.err != nil {
			t.Fatalf("read: %v", got.err)
		}
		// A notification has no id.
		if len(got.resp.ID) != 0 && string(got.resp.ID) != "null" {
			t.Fatalf("expected a notification (no id), got id=%q", got.resp.ID)
		}
	}
}

func TestCaptureSkipsWithoutTmux(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err == nil {
		t.Skip("tmux present; this test asserts the no-session fallback")
	}
	h := newHarness(t, makeWorkspace(t))
	resp := h.call(1, "capture", `{"slice":"checkout","lines":10}`)
	var got captureResult
	decodeResult(t, resp, &got)
	if len(got.Lines) != 0 {
		t.Errorf("capture lines = %v, want empty when no tmux session", got.Lines)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestReviewsReadsPendingComments(t *testing.T) {
	ws := makeWorkspace(t)
	h := newHarness(t, ws)

	store := review.Open(h.sp.Reviews)
	if _, err := store.Add(review.Comment{Slice: "checkout", Repo: "web", File: "a.go", Line: 10, Body: "fix this"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := store.Add(review.Comment{Slice: "other", Repo: "ops", File: "b.go", Line: 3, Body: "and this"}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// No slice → all pending comments.
	var all []review.Comment
	decodeResult(t, h.call(1, "reviews", ""), &all)
	if len(all) != 2 {
		t.Fatalf("reviews (all) = %d, want 2", len(all))
	}

	// Filtered to one slice.
	var one []review.Comment
	decodeResult(t, h.call(2, "reviews", `{"slice":"checkout"}`), &one)
	if len(one) != 1 || one[0].Slice != "checkout" || one[0].Body != "fix this" {
		t.Errorf("reviews (checkout) = %+v, want the one checkout comment", one)
	}
}

func TestReviewsEmptyIsArray(t *testing.T) {
	h := newHarness(t, makeWorkspace(t))
	resp := h.call(1, "reviews", "")
	var got []review.Comment
	decodeResult(t, resp, &got)
	if got == nil {
		t.Error("reviews returned null, want []")
	}
	if len(got) != 0 {
		t.Errorf("reviews = %v, want empty", got)
	}
}
