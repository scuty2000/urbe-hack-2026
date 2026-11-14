/*
Package trace appends model outputs and tool executions to a JSONL file, one
JSON object per line, so a rejected conversation can be inspected afterwards.

The recorder is a debugging aid: it is best-effort, never fails a request and
is a no-op when nil. The file lives under the storage data directory, which is
a bind mount in Docker Compose, so the trace survives container restarts. It
contains searcher messages and sanitized item attributes: keep it local and
never serve it over HTTP.
*/
package trace

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Event kinds written to the trace.
const (
	KindChatTurn    = "chat_turn"    // the searcher messages that opened a turn
	KindModelOutput = "model_output" // one provider completion (DeepSeek, D3)
	KindToolCall    = "tool_call"    // one server-side tool execution
	KindDecision    = "decision"     // one ownership evaluation
	KindReply       = "reply"        // the reply returned to the searcher
)

// Event is one JSONL line. Empty fields are omitted.
type Event struct {
	Time string `json:"time"`
	// TurnID groups every event produced by the same chat turn or request.
	TurnID string `json:"turnId,omitempty"`
	Kind   string `json:"kind"`
	// Source is the component that produced the event (agent, wizard, d3, ...).
	Source string `json:"source,omitempty"`
	Model  string `json:"model,omitempty"`
	// Tool, Arguments and Result describe a tool execution.
	Tool      string `json:"tool,omitempty"`
	Arguments any    `json:"arguments,omitempty"`
	Result    any    `json:"result,omitempty"`
	// Output is the model output or the final reply.
	Output     any    `json:"output,omitempty"`
	Error      string `json:"error,omitempty"`
	DurationMS int64  `json:"durationMs,omitempty"`
	// Data carries kind-specific details (answers, probabilities, ...).
	Data any `json:"data,omitempty"`
}

// Recorder appends events to a JSONL file. A nil Recorder discards them.
type Recorder struct {
	mu   sync.Mutex
	file *os.File
}

// Open creates the parent directory and opens path for appending.
func Open(path string) (*Recorder, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("trace: creating directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("trace: opening %q: %w", path, err)
	}
	return &Recorder{file: file}, nil
}

// Record appends one event. The turn id is read from ctx when the event does
// not carry one. Encoding and write errors are dropped on purpose.
func (r *Recorder) Record(ctx context.Context, event Event) {
	if r == nil || r.file == nil {
		return
	}
	if event.Time == "" {
		event.Time = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if event.TurnID == "" {
		event.TurnID = TurnID(ctx)
	}
	line, err := json.Marshal(event)
	if err != nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	_, _ = r.file.Write(append(line, '\n'))
}

// Close closes the underlying file.
func (r *Recorder) Close() error {
	if r == nil || r.file == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.file.Close()
}

type turnKey struct{}

// WithTurnID returns a context carrying the id that groups a turn's events.
func WithTurnID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, turnKey{}, id)
}

// TurnID returns the turn id stored in ctx, or "".
func TurnID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	id, _ := ctx.Value(turnKey{}).(string)
	return id
}

// RawJSON returns raw as a JSON value when it is valid JSON and as a plain
// string otherwise, so tool arguments and results stay readable in the trace.
func RawJSON(raw string) any {
	if raw == "" {
		return nil
	}
	if json.Valid([]byte(raw)) {
		return json.RawMessage(raw)
	}
	return raw
}

// ErrString renders an error for an event, or "" for nil.
func ErrString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
