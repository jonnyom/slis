// Package rpcserver implements `slis rpc`: a long-lived, strictly read-only
// JSON-RPC 2.0 sidecar over stdio with NDJSON framing (one JSON object per
// line). It reuses the internal read builders directly so its results are
// byte-for-byte the same shapes as the `slis <cmd> --json` commands, and pushes
// sessionEvent notifications when a slice's Claude session status changes.
//
// It never mutates a repo: mutations remain one-shot `slis <cmd>` spawns on the
// client side, out of this surface.
package rpcserver

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/fsnotify/fsnotify"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/internal/notify"
	"github.com/jonnyom/slis/internal/report"
)

// gateConcurrency caps how many subprocess-heavy methods (git / gt / gh / tmux /
// proc) run at once, mirroring the TUI's bgConcurrency so a burst of client
// requests cannot saturate the machine.
const gateConcurrency = 4

// maxLine is the largest single request line the reader accepts. Requests are
// tiny; this only guards against a pathological client.
const maxLine = 4 << 20

// Server is a read-only JSON-RPC handler bound to one workspace. It is safe for
// concurrent use: handlers run in their own goroutines and all stdout writes are
// serialised through a single mutex.
type Server struct {
	ws      config.Workspace
	sp      config.Paths
	version string

	out io.Writer
	mu  sync.Mutex // serialises writes to out (one line per message)

	gate chan struct{}

	// inFlight maps a request id (its raw JSON form) to the cancel func of that
	// request's context, so a `cancel` notification — or shutdown — stops the work
	// instead of letting it churn through subprocesses nobody will read.
	//
	// Ids are the client's to choose and may be REUSED once a request has been
	// answered, so each registration carries a monotonic seq: cleanup only ever
	// removes (and cancels) its own registration, never a newer request that
	// happens to share the id.
	inFlightMu  sync.Mutex
	inFlight    map[string]inFlightEntry
	inFlightSeq uint64

	// testHook replaces the body of the testBlock method. Tests use it to hold a
	// request open long enough to cancel it; nil in production.
	testHook func(context.Context) *rpcError

	diffCacheMu sync.Mutex
	diffCache   map[diffCacheKey]diffCacheEntry
	diffBuild   func(context.Context, model.Slice, string, string) (report.DiffResult, error)
}

// inFlightEntry is one tracked request: its cancel func plus the seq that
// identifies this particular registration of a (possibly reused) id.
type inFlightEntry struct {
	seq    uint64
	cancel context.CancelFunc
}

type diffCacheKey struct {
	slice  string
	scope  string
	format string
}

type diffCacheEntry struct {
	fingerprint string
	result      report.DiffResult
}

// New returns a Server for the given workspace and state paths. version is
// reported verbatim by the hello method.
func New(ws config.Workspace, sp config.Paths, version string) *Server {
	return &Server{
		ws:        ws,
		sp:        sp,
		version:   version,
		gate:      make(chan struct{}, gateConcurrency),
		inFlight:  make(map[string]inFlightEntry),
		diffCache: make(map[diffCacheKey]diffCacheEntry),
		diffBuild: report.SliceDiffScopedCtx,
	}
}

// Serve runs the read-dispatch loop over in/out until stdin reaches EOF or ctx
// is cancelled (SIGINT/SIGTERM). It returns nil on a clean shutdown and the
// scanner error otherwise. In-flight handlers are awaited before returning.
func (s *Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	s.out = out

	stopWatch := s.startWatcher(ctx)
	defer stopWatch()

	lines := make(chan []byte)
	scanErr := make(chan error, 1)
	go func() {
		sc := bufio.NewScanner(in)
		sc.Buffer(make([]byte, 0, 64*1024), maxLine)
		for sc.Scan() {
			line := append([]byte(nil), sc.Bytes()...)
			select {
			case lines <- line:
			case <-ctx.Done():
				return
			}
		}
		scanErr <- sc.Err()
		close(lines)
	}()

	var wg sync.WaitGroup
	// Cancel every in-flight request before waiting on the handlers: on shutdown
	// their subprocesses must die rather than run to completion with nobody left
	// to read the answers.
	defer wg.Wait()
	defer s.cancelAllInFlight()

	for {
		select {
		case <-ctx.Done():
			return nil
		case line, ok := <-lines:
			if !ok {
				return <-scanErr
			}
			trimmed := bytes.TrimSpace(line)
			if len(trimmed) == 0 {
				continue
			}
			var req request
			if err := json.Unmarshal(trimmed, &req); err != nil {
				s.writeMessage(response{JSONRPC: "2.0", Error: &rpcError{Code: codeParse, Message: "parse error"}})
				continue
			}
			// Parsing, cancellation and registration all happen HERE, on the reader
			// goroutine, so they stay in wire order: a `cancel` can never overtake the
			// registration of the request it names and be dropped as unknown.
			if req.Method == "cancel" {
				s.cancelRequest(req.Params)
				continue
			}
			reqCtx := ctx
			id := string(req.ID)
			var seq uint64
			tracked := !req.isNotification()
			if tracked {
				var cancel context.CancelFunc
				reqCtx, cancel = context.WithCancel(ctx)
				seq = s.trackInFlight(id, cancel)
			}
			wg.Add(1)
			go func(req request, reqCtx context.Context, id string, seq uint64, tracked bool) {
				defer wg.Done()
				if tracked {
					defer s.forgetInFlight(id, seq)
				}
				s.handleRequest(reqCtx, req)
			}(req, reqCtx, id, seq, tracked)
		}
	}
}

// handleRequest runs one already-parsed request under its request-scoped context
// and, unless it is a notification, writes a single response.
func (s *Server) handleRequest(ctx context.Context, req request) {
	result, rerr := s.dispatch(ctx, req)

	if req.isNotification() {
		return
	}

	resp := response{JSONRPC: "2.0", ID: req.ID}
	if rerr != nil {
		resp.Error = rerr
	} else {
		resp.Result = result
	}
	s.writeMessage(resp)
}

// trackInFlight registers a request's cancel func under its raw id and returns
// the seq identifying this registration. A newer request may legitimately reuse an
// id whose request has been answered (cleanup is deferred past the response, so
// refusing duplicates here would reject honest clients) — the seq is what keeps
// that safe: see forgetInFlight.
//
// `cancel` addresses a request by id, so it always applies to whatever is live
// under that id when the notification is read. A client that reuses ids AND
// withdraws requests it has already been answered could therefore cancel the
// wrong one; the contract in docs/AGENT.md is session-unique ids, which slis's own
// client satisfies by simply incrementing forever.
func (s *Server) trackInFlight(id string, cancel context.CancelFunc) uint64 {
	s.inFlightMu.Lock()
	defer s.inFlightMu.Unlock()
	s.inFlightSeq++
	s.inFlight[id] = inFlightEntry{seq: s.inFlightSeq, cancel: cancel}
	return s.inFlightSeq
}

// forgetInFlight drops a finished request's registration and releases its
// context. A registration that has already been replaced by a newer request
// reusing the id is left alone — cancelling it would kill live work.
func (s *Server) forgetInFlight(id string, seq uint64) {
	s.inFlightMu.Lock()
	entry, present := s.inFlight[id]
	mine := present && entry.seq == seq
	if mine {
		delete(s.inFlight, id)
	}
	s.inFlightMu.Unlock()
	if mine {
		entry.cancel()
	}
}

// cancelRequest handles a `cancel` notification: {"id": <request id>}. An id that
// is no longer in flight (a cancel racing a completed request) is ignored — the
// normal outcome, not an error.
func (s *Server) cancelRequest(raw json.RawMessage) {
	var p cancelParams
	if len(raw) == 0 || json.Unmarshal(raw, &p) != nil || len(p.ID) == 0 {
		return
	}
	s.inFlightMu.Lock()
	entry, present := s.inFlight[string(p.ID)]
	s.inFlightMu.Unlock()
	if present {
		entry.cancel()
	}
}

// cancelAllInFlight cancels every tracked request (shutdown).
func (s *Server) cancelAllInFlight() {
	s.inFlightMu.Lock()
	cancels := make([]context.CancelFunc, 0, len(s.inFlight))
	for _, entry := range s.inFlight {
		cancels = append(cancels, entry.cancel)
	}
	s.inFlightMu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
}

// inFlightCount reports how many requests are currently tracked (tests).
func (s *Server) inFlightCount() int {
	s.inFlightMu.Lock()
	defer s.inFlightMu.Unlock()
	return len(s.inFlight)
}

// dispatch routes a request to its handler. Subprocess-heavy methods run behind
// the concurrency gate; hello and other cheap file reads do not.
func (s *Server) dispatch(ctx context.Context, req request) (interface{}, *rpcError) {
	switch req.Method {
	case "hello":
		return s.hello()
	case "testBlock":
		if s.testHook == nil {
			return nil, &rpcError{Code: codeMethodNotFound, Message: "method not found: testBlock"}
		}
		return s.gated(ctx, func() (interface{}, *rpcError) { return nil, s.testHook(ctx) })
	case "ls":
		return s.gated(ctx, func() (interface{}, *rpcError) { return s.ls(ctx) })
	case "show":
		return s.gated(ctx, func() (interface{}, *rpcError) { return s.show(ctx, req.Params) })
	case "status":
		return s.gated(ctx, func() (interface{}, *rpcError) { return s.status(ctx, req.Params) })
	case "prStack":
		return s.gated(ctx, func() (interface{}, *rpcError) { return s.prStack(ctx, req.Params) })
	case "ciLog":
		return s.gated(ctx, func() (interface{}, *rpcError) { return s.ciLog(ctx, req.Params) })
	case "comments":
		return s.comments(req.Params)
	case "reviews":
		return s.reviews(req.Params)
	case "conflicts":
		return s.gated(ctx, func() (interface{}, *rpcError) { return s.conflicts(ctx) })
	case "diff":
		return s.gated(ctx, func() (interface{}, *rpcError) { return s.diff(ctx, req.Params) })
	case "branchDiff":
		return s.gated(ctx, func() (interface{}, *rpcError) { return s.branchDiff(ctx, req.Params) })
	case "tree":
		return s.gated(ctx, func() (interface{}, *rpcError) { return s.tree(ctx, req.Params) })
	case "file":
		return s.gated(ctx, func() (interface{}, *rpcError) { return s.file(ctx, req.Params) })
	case "capture":
		return s.gated(ctx, func() (interface{}, *rpcError) { return s.capture(ctx, req.Params) })
	case "procs":
		return s.gated(ctx, func() (interface{}, *rpcError) { return s.procs(ctx, req.Params) })
	default:
		return nil, &rpcError{Code: codeMethodNotFound, Message: "method not found: " + req.Method}
	}
}

// gated runs fn while holding one of the gateConcurrency slots. Waiting for a
// slot is cancellable: a client that gives up on a queued read never has its work
// started, so a burst of abandoned requests cannot keep spawning subprocesses.
func (s *Server) gated(ctx context.Context, fn func() (interface{}, *rpcError)) (interface{}, *rpcError) {
	if ctx.Err() != nil {
		return nil, cancelledErr()
	}
	select {
	case s.gate <- struct{}{}:
	case <-ctx.Done():
		return nil, cancelledErr()
	}
	defer func() { <-s.gate }()

	result, rerr := fn()
	// A read whose subprocesses were killed mid-flight returns whatever it managed
	// to collect (the radar, for instance, degrades to per-scope "incomplete"
	// markers). That partial view must never be handed back as an answer — a
	// half-read conflict radar reads like "no conflicts". Report the cancellation.
	if ctx.Err() != nil {
		return nil, cancelledErr()
	}
	return result, rerr
}

// writeMessage marshals v to one NDJSON line and writes it under the stdout
// mutex, so concurrent handlers never interleave output.
func (s *Server) writeMessage(v interface{}) {
	b, err := json.Marshal(v)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rpc: marshal:", err)
		return
	}
	b = append(b, '\n')
	s.mu.Lock()
	defer s.mu.Unlock()
	_, _ = s.out.Write(b)
}

// startWatcher watches the notify events dir and pushes a sessionEvent whenever
// a slice's status changes. It returns a stop func; if the watcher cannot be
// created (no fsnotify, missing dir) it logs to stderr and returns a no-op stop.
func (s *Server) startWatcher(ctx context.Context) func() {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		fmt.Fprintln(os.Stderr, "rpc: session watcher unavailable:", err)
		return func() {}
	}
	if err := w.Add(s.sp.EventsDir); err != nil {
		fmt.Fprintln(os.Stderr, "rpc: cannot watch events dir:", err)
		_ = w.Close()
		return func() {}
	}

	watcherDone := make(chan struct{})
	go func() {
		defer close(watcherDone)
		prev := notify.ReadAllStatuses(s.sp.EventsDir)
		for {
			select {
			case <-ctx.Done():
				return
			case _, ok := <-w.Events:
				if !ok {
					return
				}
				prev = s.emitChangedStatuses(prev)
			case _, ok := <-w.Errors:
				if !ok {
					return
				}
			}
		}
	}()

	// Closing the watcher unblocks the goroutine's select; waiting on watcherDone
	// guarantees it has stopped reading the events dir before Serve returns.
	return func() {
		_ = w.Close()
		<-watcherDone
	}
}

// emitChangedStatuses re-reads the event store, emits a sessionEvent for every
// slice whose status differs from prev (a removed file reads as "none"), and
// returns the new snapshot.
func (s *Server) emitChangedStatuses(prev map[string]model.SessionStatus) map[string]model.SessionStatus {
	cur := notify.ReadAllStatuses(s.sp.EventsDir)
	for slice, st := range cur {
		if prev[slice] != st {
			s.emitSessionEvent(slice, st.String())
		}
	}
	for slice := range prev {
		if _, still := cur[slice]; !still {
			s.emitSessionEvent(slice, model.SessNone.String())
		}
	}
	return cur
}

// emitSessionEvent pushes a single sessionEvent notification to the client.
func (s *Server) emitSessionEvent(slice, status string) {
	s.writeMessage(notification{
		JSONRPC: "2.0",
		Method:  "sessionEvent",
		Params:  sessionEventParams{Slice: slice, Status: status},
	})
}
