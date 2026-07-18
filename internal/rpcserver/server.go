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
}

// New returns a Server for the given workspace and state paths. version is
// reported verbatim by the hello method.
func New(ws config.Workspace, sp config.Paths, version string) *Server {
	return &Server{
		ws:      ws,
		sp:      sp,
		version: version,
		gate:    make(chan struct{}, gateConcurrency),
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
	defer wg.Wait()

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
			wg.Add(1)
			go func(raw []byte) {
				defer wg.Done()
				s.handleLine(ctx, raw)
			}(trimmed)
		}
	}
}

// handleLine parses one request line and, unless it is a notification, writes a
// single response.
func (s *Server) handleLine(ctx context.Context, raw []byte) {
	var req request
	if err := json.Unmarshal(raw, &req); err != nil {
		s.writeMessage(response{JSONRPC: "2.0", Error: &rpcError{Code: codeParse, Message: "parse error"}})
		return
	}

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

// dispatch routes a request to its handler. Subprocess-heavy methods run behind
// the concurrency gate; hello and other cheap file reads do not.
func (s *Server) dispatch(ctx context.Context, req request) (interface{}, *rpcError) {
	switch req.Method {
	case "hello":
		return s.hello()
	case "ls":
		return s.gated(s.ls)
	case "show":
		return s.gated(func() (interface{}, *rpcError) { return s.show(req.Params) })
	case "status":
		return s.gated(func() (interface{}, *rpcError) { return s.status(req.Params) })
	case "prStack":
		return s.gated(func() (interface{}, *rpcError) { return s.prStack(req.Params) })
	case "ciLog":
		return s.gated(func() (interface{}, *rpcError) { return s.ciLog(req.Params) })
	case "comments":
		return s.comments(req.Params)
	case "reviews":
		return s.reviews(req.Params)
	case "conflicts":
		return s.gated(s.conflicts)
	case "diff":
		return s.gated(func() (interface{}, *rpcError) { return s.diff(req.Params) })
	case "branchDiff":
		return s.gated(func() (interface{}, *rpcError) { return s.branchDiff(req.Params) })
	case "tree":
		return s.gated(func() (interface{}, *rpcError) { return s.tree(req.Params) })
	case "file":
		return s.gated(func() (interface{}, *rpcError) { return s.file(req.Params) })
	case "capture":
		return s.gated(func() (interface{}, *rpcError) { return s.capture(req.Params) })
	case "procs":
		return s.gated(func() (interface{}, *rpcError) { return s.procs(req.Params) })
	default:
		return nil, &rpcError{Code: codeMethodNotFound, Message: "method not found: " + req.Method}
	}
}

// gated runs fn while holding one of the gateConcurrency slots.
func (s *Server) gated(fn func() (interface{}, *rpcError)) (interface{}, *rpcError) {
	s.gate <- struct{}{}
	defer func() { <-s.gate }()
	return fn()
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
