package rpcserver

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jonnyom/slis/internal/config"
)

// syncWriter collects NDJSON lines written by the server.
type syncWriter struct {
	mu    sync.Mutex
	lines []string
}

func (w *syncWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, line := range strings.Split(strings.TrimRight(string(p), "\n"), "\n") {
		if line != "" {
			w.lines = append(w.lines, line)
		}
	}
	return len(p), nil
}

func (w *syncWriter) snapshot() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]string(nil), w.lines...)
}

// waitForResponse polls until a response carrying id appears, or fails.
func waitForResponse(t *testing.T, w *syncWriter, id string, within time.Duration) response {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		for _, line := range w.snapshot() {
			var resp response
			if err := json.Unmarshal([]byte(line), &resp); err != nil {
				continue
			}
			if string(resp.ID) == id {
				return resp
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("no response for id %s within %s; got %v", id, within, w.snapshot())
	return response{}
}

// blockingServer returns a server whose gated methods are all held by a stub that
// blocks until released, plus the release func. The stub stands in for a real
// subprocess fan-out (conflicts/ls) that is slow enough to be cancelled.
func blockingServer(t *testing.T) (*Server, *syncWriter, func()) {
	t.Helper()
	sp := config.Paths{EventsDir: t.TempDir()}
	s := New(config.Workspace{}, sp, "test")

	release := make(chan struct{})
	var once sync.Once
	s.testHook = func(ctx context.Context) *rpcError {
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return cancelledErr()
		}
	}
	return s, &syncWriter{}, func() { once.Do(func() { close(release) }) }
}

// TestCancelStopsAQueuedRequest is the point of the protocol: a client that gives
// up on a read must be able to stop the work, instead of leaving the sidecar
// churning through subprocesses nobody will read.
func TestCancelStopsAQueuedRequest(t *testing.T) {
	s, out, release := blockingServer(t)
	defer release()

	in, writeLine := requestPipe(t)
	ctx, stop := context.WithCancel(context.Background())
	defer stop()

	served := make(chan error, 1)
	go func() { served <- s.Serve(ctx, in, out) }()

	writeLine(`{"jsonrpc":"2.0","id":7,"method":"testBlock"}`)
	waitForInFlight(t, s, 1, 2*time.Second)
	writeLine(`{"jsonrpc":"2.0","method":"cancel","params":{"id":7}}`)

	resp := waitForResponse(t, out, "7", 2*time.Second)
	if resp.Error == nil || resp.Error.Code != codeRequestCancelled {
		t.Fatalf("want a request-cancelled error, got %+v", resp)
	}
	if inFlight := s.inFlightCount(); inFlight != 0 {
		t.Fatalf("cancelled request still tracked: %d in flight", inFlight)
	}

	stop()
	<-served
}

// TestCancelImmediatelyAfterRequestIsNotDropped pins the ordering guarantee: a
// client that fires request-then-cancel back to back must have the cancel land,
// even though the handler runs on its own goroutine and may not have started yet.
// Registration happens on the reader goroutine, in wire order, so the cancel
// cannot overtake it and be ignored as an unknown id.
func TestCancelImmediatelyAfterRequestIsNotDropped(t *testing.T) {
	s, out, release := blockingServer(t)
	defer release()

	in, writeLine := requestPipe(t)
	ctx, stop := context.WithCancel(context.Background())
	defer stop()

	served := make(chan error, 1)
	go func() { served <- s.Serve(ctx, in, out) }()

	writeLine(`{"jsonrpc":"2.0","id":3,"method":"testBlock"}`)
	writeLine(`{"jsonrpc":"2.0","method":"cancel","params":{"id":3}}`)

	resp := waitForResponse(t, out, "3", 2*time.Second)
	if resp.Error == nil || resp.Error.Code != codeRequestCancelled {
		t.Fatalf("want a request-cancelled error, got %+v", resp)
	}

	stop()
	<-served
}

// TestCancelIsScopedToOneRequest guards against a cancel taking out unrelated
// in-flight reads.
func TestCancelIsScopedToOneRequest(t *testing.T) {
	s, out, release := blockingServer(t)
	defer release()

	in, writeLine := requestPipe(t)
	ctx, stop := context.WithCancel(context.Background())
	defer stop()

	served := make(chan error, 1)
	go func() { served <- s.Serve(ctx, in, out) }()

	writeLine(`{"jsonrpc":"2.0","id":1,"method":"testBlock"}`)
	writeLine(`{"jsonrpc":"2.0","id":2,"method":"testBlock"}`)
	waitForInFlight(t, s, 2, 2*time.Second)

	writeLine(`{"jsonrpc":"2.0","method":"cancel","params":{"id":1}}`)
	if resp := waitForResponse(t, out, "1", 2*time.Second); resp.Error == nil {
		t.Fatalf("request 1 should have been cancelled, got %+v", resp)
	}

	release()
	if resp := waitForResponse(t, out, "2", 2*time.Second); resp.Error != nil {
		t.Fatalf("request 2 should have completed, got error %+v", resp.Error)
	}

	stop()
	<-served
}

// TestCancelledRequestNeverReturnsPartialData is the safety property: a read whose
// subprocesses were killed mid-flight collects only part of the picture (the
// radar degrades to "incomplete" markers, which a UI could render as "no
// conflicts"). The server must answer with the cancellation, not that partial view.
func TestCancelledRequestNeverReturnsPartialData(t *testing.T) {
	sp := config.Paths{EventsDir: t.TempDir()}
	s := New(config.Workspace{}, sp, "test")
	out := &syncWriter{}

	started := make(chan struct{})
	// A handler that ignores cancellation and "succeeds" with partial data.
	s.testHook = func(context.Context) *rpcError {
		close(started)
		time.Sleep(150 * time.Millisecond)
		return nil
	}

	in, writeLine := requestPipe(t)
	ctx, stop := context.WithCancel(context.Background())
	defer stop()

	served := make(chan error, 1)
	go func() { served <- s.Serve(ctx, in, out) }()

	writeLine(`{"jsonrpc":"2.0","id":11,"method":"testBlock"}`)
	<-started
	writeLine(`{"jsonrpc":"2.0","method":"cancel","params":{"id":11}}`)

	resp := waitForResponse(t, out, "11", 3*time.Second)
	if resp.Error == nil || resp.Error.Code != codeRequestCancelled {
		t.Fatalf("partial result leaked as an answer: %+v", resp)
	}

	stop()
	<-served
}

// TestShutdownCancelsInFlightRequests keeps a quitting front-end from leaving the
// sidecar's subprocess work running behind it.
func TestShutdownCancelsInFlightRequests(t *testing.T) {
	s, out, release := blockingServer(t)
	defer release()

	in, writeLine := requestPipe(t)
	ctx, stop := context.WithCancel(context.Background())

	served := make(chan error, 1)
	go func() { served <- s.Serve(ctx, in, out) }()

	writeLine(`{"jsonrpc":"2.0","id":9,"method":"testBlock"}`)
	waitForInFlight(t, s, 1, 2*time.Second)

	stop()
	select {
	case <-served:
	case <-time.After(3 * time.Second):
		t.Fatal("Serve did not return after shutdown — in-flight work was not cancelled")
	}
	if inFlight := s.inFlightCount(); inFlight != 0 {
		t.Fatalf("%d requests still tracked after shutdown", inFlight)
	}
}

// TestCancelUnknownIDIsIgnored: a cancel racing a just-finished request is normal
// and must not error or panic.
func TestCancelUnknownIDIsIgnored(t *testing.T) {
	s, out, release := blockingServer(t)
	defer release()

	in, writeLine := requestPipe(t)
	ctx, stop := context.WithCancel(context.Background())
	defer stop()

	served := make(chan error, 1)
	go func() { served <- s.Serve(ctx, in, out) }()

	writeLine(`{"jsonrpc":"2.0","method":"cancel","params":{"id":404}}`)
	writeLine(`{"jsonrpc":"2.0","id":5,"method":"hello"}`)

	if resp := waitForResponse(t, out, "5", 2*time.Second); resp.Error != nil {
		t.Fatalf("hello after a stray cancel failed: %+v", resp.Error)
	}

	stop()
	<-served
}

// requestPipe returns a reader the server can Serve from plus a func that feeds it
// one NDJSON line.
func requestPipe(t *testing.T) (io.Reader, func(string)) {
	t.Helper()
	r, w := io.Pipe()
	t.Cleanup(func() { _ = w.Close() })
	return r, func(line string) {
		if _, err := io.WriteString(w, line+"\n"); err != nil {
			t.Errorf("write request: %v", err)
		}
	}
}

func waitForInFlight(t *testing.T, s *Server, want int, within time.Duration) {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if s.inFlightCount() == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("in-flight requests = %d, want %d", s.inFlightCount(), want)
}

// TestReusedRequestIDIsNotCancelledByItsPredecessor pins the id-reuse rule. A
// client may reuse an id once the previous request was answered, and that
// request's cleanup runs after its response is written — so the cleanup must
// never cancel the newer request now holding the id.
func TestReusedRequestIDIsNotCancelledByItsPredecessor(t *testing.T) {
	h := newHarness(t, config.Workspace{})

	for i := 0; i < 5; i++ {
		resp := h.call(1, "hello", "")
		if resp.Error != nil {
			t.Fatalf("call %d with reused id 1 failed: %+v", i, resp.Error)
		}
	}
}
