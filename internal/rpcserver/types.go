package rpcserver

import "encoding/json"

// JSON-RPC 2.0 error codes. The negative -320xx range is reserved by the spec;
// -32000 is the generic slis server error, carrying a data.kind for known error
// kinds (e.g. "slice-not-found").
const (
	codeParse          = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeServer         = -32000
)

// request is an incoming JSON-RPC 2.0 request. ID is left raw so a client may
// use a number, a string, or omit it (a notification, which we do not reply to).
type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

// isNotification reports whether the request omitted its id (a JSON-RPC
// notification the server must not reply to).
func (r request) isNotification() bool {
	return len(r.ID) == 0 || string(r.ID) == "null"
}

// response is an outgoing JSON-RPC 2.0 response. Exactly one of Result/Error is
// set; the other is nil and omitted. Result values are always non-nil structs,
// so omitempty never drops a legitimate result.
type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// rpcError is a JSON-RPC 2.0 error object. Data carries the slis error kind when
// one is known, so a client can branch on kind rather than parse the message.
type rpcError struct {
	Code    int      `json:"code"`
	Message string   `json:"message"`
	Data    *errData `json:"data,omitempty"`
}

// errData carries the machine-readable slis error kind.
type errData struct {
	Kind string `json:"kind,omitempty"`
}

// serverErr builds a -32000 slis error carrying an optional kind.
func serverErr(msg, kind string) *rpcError {
	e := &rpcError{Code: codeServer, Message: msg}
	if kind != "" {
		e.Data = &errData{Kind: kind}
	}
	return e
}

// notification is a server → client push with no id (JSON-RPC notification).
type notification struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
}

// helloResult is the `hello` method's result.
type helloResult struct {
	Version       string         `json:"version"`
	WorkspaceRoot string         `json:"workspaceRoot"`
	Sessions      sessionsResult `json:"sessions"`
}

// sessionsResult surfaces the workspace's session config so the JS front-end can
// build session-launch options (harness/agent/layout/autostart) instead of
// hardcoding the Go-TUI defaults. Harness and Agent are the resolved values
// (config helpers apply the "claude" defaults); Layout is raw ("" means the
// front-end applies its own root-vs-repos default); Autostart already has the
// legacy autostart_claude alias merged in on load.
type sessionsResult struct {
	Harness   string `json:"harness"`
	Agent     string `json:"agent"`
	Layout    string `json:"layout"`
	Autostart bool   `json:"autostart"`
	// Editor is the configured editor binary (workspace.yaml sessions.editor),
	// "" when unset — the front-end's e/o keys use it to skip the editor picker.
	Editor string `json:"editor,omitempty"`
}

// sliceParams is the shared param shape for methods that name a slice.
type sliceParams struct {
	Slice string `json:"slice"`
}

// optionalSliceParams is used by methods where the slice is optional (status,
// procs): an empty slice means "all slices".
type optionalSliceParams struct {
	Slice string `json:"slice"`
}

// diffParams selects the diff scope and format for the `diff` method.
type diffParams struct {
	Slice  string `json:"slice"`
	Scope  string `json:"scope"`
	Format string `json:"format"`
}

// ciLogParams names a slice and, optionally, a single repo to fetch the failing
// CI log for. An empty repo fetches every member repo that has a PR.
type ciLogParams struct {
	Slice string `json:"slice"`
	Repo  string `json:"repo"`
}

// ciLogRepoResult is one repo's failing-CI log excerpt. Exactly one of Log/Error
// is set: Log carries the safeterm-stripped `gh run view --log-failed` output;
// Error explains why no log is available (no PR, no failing run, gh absent).
type ciLogRepoResult struct {
	Repo   string `json:"repo"`
	Branch string `json:"branch"`
	Log    string `json:"log,omitempty"`
	Error  string `json:"error,omitempty"`
}

// ciLogResult is the `ciLog` method's result: one entry per target repo.
type ciLogResult struct {
	Repos []ciLogRepoResult `json:"repos"`
}

// captureParams selects how many trailing lines the `capture` method returns.
type captureParams struct {
	Slice string `json:"slice"`
	Lines int    `json:"lines"`
}

// captureResult is the `capture` method's result: the safeterm-stripped tail of
// the slice's tmux session.
type captureResult struct {
	Lines []string `json:"lines"`
}

// procResult is a single process in a slice's session.
type procResult struct {
	PID  int     `json:"pid"`
	PPID int     `json:"ppid"`
	Cmd  string  `json:"cmd"`
	CPU  float64 `json:"cpu"`
	Mem  float64 `json:"mem"`
}

// sliceProcsResult holds one slice's processes plus its total CPU.
type sliceProcsResult struct {
	Slice    string       `json:"slice"`
	Procs    []procResult `json:"procs"`
	TotalCPU float64      `json:"totalCPU"`
}

// procsResult is the `procs` method's result: one entry per slice sampled.
type procsResult struct {
	Slices []sliceProcsResult `json:"slices"`
}

// sessionEventParams is the payload of a sessionEvent notification.
type sessionEventParams struct {
	Slice  string `json:"slice"`
	Status string `json:"status"`
}
