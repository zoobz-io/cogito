package cogito

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/zoobz-io/zyn"
)

// mockToolExecutor provides a canned set of tools and results for testing.
type mockToolExecutor struct {
	tools   []zyn.Tool
	results map[string]string // tool name → result
	errors  map[string]error  // tool name → error (optional)
	calls   []zyn.ToolCall    // records all calls made
}

func (m *mockToolExecutor) ListTools() []zyn.Tool {
	return m.tools
}

func (m *mockToolExecutor) Execute(_ context.Context, call zyn.ToolCall) (string, error) {
	m.calls = append(m.calls, call)
	if m.errors != nil {
		if err, ok := m.errors[call.Name]; ok {
			return "", err
		}
	}
	if result, ok := m.results[call.Name]; ok {
		return result, nil
	}
	return "unknown tool", nil
}

// mockEngageProvider implements Provider and ToolProvider with configurable responses.
type mockEngageProvider struct {
	callCount int
	responses []*zyn.ProviderResponse // sequence of responses to return
}

func (m *mockEngageProvider) Call(_ context.Context, _ []zyn.Message, _ float32) (*zyn.ProviderResponse, error) {
	return nil, fmt.Errorf("engage should use CallWithTools, not Call")
}

func (m *mockEngageProvider) Name() string {
	return "mock-engage"
}

func (m *mockEngageProvider) CallWithTools(_ context.Context, _ []zyn.Message, _ float32, _ []zyn.Tool) (*zyn.ProviderResponse, error) {
	if m.callCount >= len(m.responses) {
		return nil, fmt.Errorf("unexpected call %d (only %d responses configured)", m.callCount+1, len(m.responses))
	}
	resp := m.responses[m.callCount]
	m.callCount++
	return resp, nil
}

// mockEngageStreamingProvider adds ToolStreamingProvider to mockEngageProvider.
type mockEngageStreamingProvider struct {
	mockEngageProvider
}

func (m *mockEngageStreamingProvider) StreamWithTools(ctx context.Context, messages []zyn.Message, temperature float32, tools []zyn.Tool, callback zyn.StreamEventCallback) (*zyn.ProviderResponse, error) {
	resp, err := m.CallWithTools(ctx, messages, temperature, tools)
	if err != nil {
		return nil, err
	}
	if callback != nil {
		if resp.Content != "" {
			callback(zyn.StreamEvent{Type: zyn.StreamEventText, Text: resp.Content})
		}
		for i, tc := range resp.ToolCalls {
			tcCopy := tc
			callback(zyn.StreamEvent{Type: zyn.StreamEventToolStart, ToolCall: &tcCopy, Index: i})
			callback(zyn.StreamEvent{Type: zyn.StreamEventToolEnd, Index: i})
		}
	}
	return resp, nil
}

// mockNonToolProvider implements Provider but NOT ToolProvider.
type mockNonToolProvider struct{}

func (m *mockNonToolProvider) Call(_ context.Context, _ []zyn.Message, _ float32) (*zyn.ProviderResponse, error) {
	return &zyn.ProviderResponse{Content: "no tools"}, nil
}

func (m *mockNonToolProvider) Name() string {
	return "mock-non-tool"
}

// --- Helpers ---

func newTestExecutor() *mockToolExecutor {
	return &mockToolExecutor{
		tools: []zyn.Tool{
			{Name: "search", Description: "Search for information", Parameters: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}}}`)},
			{Name: "lookup", Description: "Look up a specific item", Parameters: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"}}}`)},
		},
		results: map[string]string{
			"search": "Found 3 results for query",
			"lookup": "Item details: name=test, status=active",
		},
	}
}

func textResponse(content string) *zyn.ProviderResponse {
	return &zyn.ProviderResponse{
		Content:    content,
		StopReason: zyn.StopReasonEndTurn,
		Usage:      zyn.TokenUsage{Prompt: 100, Completion: 50, Total: 150},
	}
}

func toolResponse(calls ...zyn.ToolCall) *zyn.ProviderResponse {
	return &zyn.ProviderResponse{
		Content:    "",
		StopReason: zyn.StopReasonToolUse,
		ToolCalls:  calls,
		Usage:      zyn.TokenUsage{Prompt: 100, Completion: 50, Total: 150},
	}
}

func tc(id, name, input string) zyn.ToolCall {
	return zyn.ToolCall{ID: id, Name: name, Input: json.RawMessage(input)}
}

// --- Tests ---

func TestEngageNoToolUse(t *testing.T) {
	provider := &mockEngageProvider{
		responses: []*zyn.ProviderResponse{
			textResponse("Direct answer without tools"),
		},
	}
	SetProvider(provider)
	defer SetProvider(nil)

	executor := newTestExecutor()
	engage := NewEngage("answer", "Answer the question", executor)

	thought := newTestThought("test no tools")
	thought.SetContent(context.Background(), "question", "What is 2+2?", "user")

	result, err := engage.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output, err := engage.Scan(result)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	if output.Content != "Direct answer without tools" {
		t.Errorf("expected direct answer, got %q", output.Content)
	}
	if output.Iterations != 1 {
		t.Errorf("expected 1 iteration, got %d", output.Iterations)
	}
	if !output.Completed {
		t.Error("expected completed to be true")
	}
	if len(output.ToolCalls) != 0 {
		t.Errorf("expected 0 tool calls, got %d", len(output.ToolCalls))
	}
}

func TestEngageSingleToolCall(t *testing.T) {
	provider := &mockEngageProvider{
		responses: []*zyn.ProviderResponse{
			toolResponse(tc("call_1", "search", `{"query":"test"}`)),
			textResponse("Based on search results: 3 items found"),
		},
	}
	SetProvider(provider)
	defer SetProvider(nil)

	executor := newTestExecutor()
	engage := NewEngage("answer", "Answer using tools", executor)

	thought := newTestThought("test single tool")
	thought.SetContent(context.Background(), "question", "Find test items", "user")

	result, err := engage.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output, err := engage.Scan(result)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	if output.Iterations != 2 {
		t.Errorf("expected 2 iterations, got %d", output.Iterations)
	}
	if len(output.ToolCalls) != 1 {
		t.Errorf("expected 1 tool call, got %d", len(output.ToolCalls))
	}
	if output.ToolCalls[0].Name != "search" {
		t.Errorf("expected tool 'search', got %q", output.ToolCalls[0].Name)
	}
	if output.ToolCalls[0].Error {
		t.Error("expected no error on tool call")
	}

	// Verify executor received the call
	if len(executor.calls) != 1 {
		t.Fatalf("expected 1 executor call, got %d", len(executor.calls))
	}
	if executor.calls[0].Name != "search" {
		t.Errorf("expected executor call to 'search', got %q", executor.calls[0].Name)
	}
}

func TestEngageMultipleToolCallsInOneResponse(t *testing.T) {
	provider := &mockEngageProvider{
		responses: []*zyn.ProviderResponse{
			toolResponse(
				tc("call_1", "search", `{"query":"test"}`),
				tc("call_2", "lookup", `{"id":"123"}`),
			),
			textResponse("Combined results"),
		},
	}
	SetProvider(provider)
	defer SetProvider(nil)

	executor := newTestExecutor()
	engage := NewEngage("answer", "Answer using tools", executor)

	thought := newTestThought("test multiple tools")
	thought.SetContent(context.Background(), "question", "Find and lookup", "user")

	result, err := engage.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output, err := engage.Scan(result)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	if len(output.ToolCalls) != 2 {
		t.Errorf("expected 2 tool calls, got %d", len(output.ToolCalls))
	}
	if len(executor.calls) != 2 {
		t.Errorf("expected 2 executor calls, got %d", len(executor.calls))
	}
}

func TestEngageMultipleIterations(t *testing.T) {
	provider := &mockEngageProvider{
		responses: []*zyn.ProviderResponse{
			toolResponse(tc("call_1", "search", `{"query":"first"}`)),
			toolResponse(tc("call_2", "search", `{"query":"second"}`)),
			toolResponse(tc("call_3", "lookup", `{"id":"456"}`)),
			textResponse("Final answer after 3 tool iterations"),
		},
	}
	SetProvider(provider)
	defer SetProvider(nil)

	executor := newTestExecutor()
	engage := NewEngage("answer", "Answer thoroughly", executor)

	thought := newTestThought("test multi iteration")
	thought.SetContent(context.Background(), "question", "Complex question", "user")

	result, err := engage.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output, err := engage.Scan(result)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	if output.Iterations != 4 {
		t.Errorf("expected 4 iterations, got %d", output.Iterations)
	}
	if len(output.ToolCalls) != 3 {
		t.Errorf("expected 3 tool calls, got %d", len(output.ToolCalls))
	}
}

func TestEngageMaxIterationsReached(t *testing.T) {
	provider := &mockEngageProvider{
		responses: []*zyn.ProviderResponse{
			toolResponse(tc("call_1", "search", `{"query":"1"}`)),
			toolResponse(tc("call_2", "search", `{"query":"2"}`)),
			toolResponse(tc("call_3", "search", `{"query":"3"}`)),
		},
	}
	SetProvider(provider)
	defer SetProvider(nil)

	executor := newTestExecutor()
	engage := NewEngage("answer", "Answer", executor).WithMaxIterations(3)

	thought := newTestThought("test max iterations")
	thought.SetContent(context.Background(), "question", "Runaway", "user")

	result, err := engage.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output, err := engage.Scan(result)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	if output.Iterations != 3 {
		t.Errorf("expected 3 iterations (max), got %d", output.Iterations)
	}
	if output.Completed {
		t.Error("expected completed to be false when max iterations exhausted")
	}
	// Content may be empty since last response was tool_use
	if output.Content != "" {
		t.Errorf("expected empty content when max iterations exhausted with tool_use, got %q", output.Content)
	}
}

func TestEngageToolError(t *testing.T) {
	provider := &mockEngageProvider{
		responses: []*zyn.ProviderResponse{
			toolResponse(tc("call_1", "search", `{"query":"fail"}`)),
			textResponse("Handled the error gracefully"),
		},
	}
	SetProvider(provider)
	defer SetProvider(nil)

	executor := &mockToolExecutor{
		tools:  []zyn.Tool{{Name: "search", Description: "Search"}},
		errors: map[string]error{"search": fmt.Errorf("connection timeout")},
	}
	engage := NewEngage("answer", "Answer", executor)

	thought := newTestThought("test tool error")
	thought.SetContent(context.Background(), "question", "Search something", "user")

	result, err := engage.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output, err := engage.Scan(result)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	if len(output.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(output.ToolCalls))
	}
	if !output.ToolCalls[0].Error {
		t.Error("expected tool call to be marked as error")
	}
	if output.ToolCalls[0].Output != "connection timeout" {
		t.Errorf("expected error message in output, got %q", output.ToolCalls[0].Output)
	}
	// Process should succeed — error fed back to LLM
	if output.Content != "Handled the error gracefully" {
		t.Errorf("expected graceful handling, got %q", output.Content)
	}
}

func TestEngageProviderNotToolProvider(t *testing.T) {
	SetProvider(&mockNonToolProvider{})
	defer SetProvider(nil)

	executor := newTestExecutor()
	engage := NewEngage("answer", "Answer", executor)
	thought := newTestThought("test not tool provider")

	_, err := engage.Process(context.Background(), thought)
	if err == nil {
		t.Fatal("expected error when provider does not support tools")
	}
	if !strings.Contains(err.Error(), "does not support tools") {
		t.Errorf("expected tool support error, got: %v", err)
	}
}

func TestEngageNoProvider(t *testing.T) {
	SetProvider(nil)

	executor := newTestExecutor()
	engage := NewEngage("answer", "Answer", executor)
	thought := newTestThought("test no provider")

	_, err := engage.Process(context.Background(), thought)
	if err == nil {
		t.Fatal("expected error when no provider configured")
	}
	if !strings.Contains(err.Error(), "no provider") {
		t.Errorf("expected no provider error, got: %v", err)
	}
}

func TestEngageProviderCallFailure(t *testing.T) {
	provider := &mockEngageProvider{
		responses: []*zyn.ProviderResponse{}, // no responses = error
	}
	SetProvider(provider)
	defer SetProvider(nil)

	executor := newTestExecutor()
	engage := NewEngage("answer", "Answer", executor)

	thought := newTestThought("test provider failure")
	thought.SetContent(context.Background(), "question", "Fail", "user")

	_, err := engage.Process(context.Background(), thought)
	if err == nil {
		t.Fatal("expected error on provider call failure")
	}
	if !strings.Contains(err.Error(), "provider call failed") {
		t.Errorf("expected provider call error, got: %v", err)
	}
}

func TestEngageNotesPublished(t *testing.T) {
	provider := &mockEngageProvider{
		responses: []*zyn.ProviderResponse{
			textResponse("Done"),
		},
	}
	SetProvider(provider)
	defer SetProvider(nil)

	executor := newTestExecutor()
	engage := NewEngage("answer", "Answer", executor)

	thought := newTestThought("test notes published")
	thought.SetContent(context.Background(), "input", "data", "test")

	result, err := engage.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	unpublished := result.GetUnpublishedNotes()
	if len(unpublished) != 0 {
		t.Errorf("expected 0 unpublished notes, got %d", len(unpublished))
	}
}

func TestEngageSessionUpdated(t *testing.T) {
	provider := &mockEngageProvider{
		responses: []*zyn.ProviderResponse{
			toolResponse(tc("call_1", "search", `{"query":"test"}`)),
			textResponse("Answer based on search"),
		},
	}
	SetProvider(provider)
	defer SetProvider(nil)

	executor := newTestExecutor()
	engage := NewEngage("answer", "Answer", executor)

	thought := newTestThought("test session updated")
	thought.SetContent(context.Background(), "question", "Find stuff", "user")

	result, err := engage.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Session should contain tool messages
	messages := result.Session.Messages()
	hasToolMessage := false
	for _, msg := range messages {
		if msg.Role == zyn.RoleTool {
			hasToolMessage = true
			break
		}
	}
	if !hasToolMessage {
		t.Error("expected session to contain tool result messages")
	}
}

func TestEngageStreamingCallback(t *testing.T) {
	provider := &mockEngageStreamingProvider{
		mockEngageProvider: mockEngageProvider{
			responses: []*zyn.ProviderResponse{
				toolResponse(tc("call_1", "search", `{"query":"test"}`)),
				textResponse("Streamed answer"),
			},
		},
	}
	SetProvider(provider)
	defer SetProvider(nil)

	executor := newTestExecutor()
	var events []zyn.StreamEvent
	engage := NewEngage("answer", "Answer", executor).
		WithStreamEventCallback(func(event zyn.StreamEvent) {
			events = append(events, event)
		})

	thought := newTestThought("test streaming")
	thought.SetContent(context.Background(), "question", "Stream it", "user")

	result, err := engage.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have received events
	if len(events) == 0 {
		t.Error("expected streaming events")
	}

	// Result should be identical
	output, err := engage.Scan(result)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}
	if output.Content != "Streamed answer" {
		t.Errorf("expected 'Streamed answer', got %q", output.Content)
	}
}

func TestEngageStreamingFallback(t *testing.T) {
	// Provider implements ToolProvider but NOT ToolStreamingProvider
	provider := &mockEngageProvider{
		responses: []*zyn.ProviderResponse{
			textResponse("Non-streaming answer"),
		},
	}
	SetProvider(provider)
	defer SetProvider(nil)

	executor := newTestExecutor()
	callbackCalled := false
	engage := NewEngage("answer", "Answer", executor).
		WithStreamEventCallback(func(_ zyn.StreamEvent) {
			callbackCalled = true
		})

	thought := newTestThought("test streaming fallback")
	thought.SetContent(context.Background(), "question", "Fallback", "user")

	result, err := engage.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Callback should NOT have been called (provider doesn't support streaming)
	if callbackCalled {
		t.Error("callback should not be called when provider doesn't support streaming")
	}

	output, err := engage.Scan(result)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}
	if output.Content != "Non-streaming answer" {
		t.Errorf("expected 'Non-streaming answer', got %q", output.Content)
	}
}

func TestEngageBuilderMethods(t *testing.T) {
	provider := &mockEngageProvider{
		responses: []*zyn.ProviderResponse{
			textResponse("Built"),
		},
	}
	SetProvider(provider)
	defer SetProvider(nil)

	executor := newTestExecutor()
	engage := NewEngage("answer", "Answer", executor).
		WithProvider(provider).
		WithTemperature(0.5).
		WithMaxIterations(5).
		WithStreamEventCallback(func(_ zyn.StreamEvent) {})

	thought := newTestThought("test builders")
	thought.SetContent(context.Background(), "input", "data", "test")

	_, err := engage.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEngageIdentity(t *testing.T) {
	executor := newTestExecutor()
	engage := NewEngage("my_engage", "prompt", executor)
	if engage.Identity().Name() != "my_engage" {
		t.Errorf("expected name 'my_engage', got %q", engage.Identity().Name())
	}
}

func TestEngageSchema(t *testing.T) {
	executor := newTestExecutor()
	engage := NewEngage("my_engage", "prompt", executor)
	schema := engage.Schema()
	if schema.Type != "engage" {
		t.Errorf("expected type 'engage', got %q", schema.Type)
	}
}

func TestEngageClose(t *testing.T) {
	executor := newTestExecutor()
	engage := NewEngage("my_engage", "prompt", executor)
	if err := engage.Close(); err != nil {
		t.Errorf("unexpected error on close: %v", err)
	}
}

func TestEngageScanNotFound(t *testing.T) {
	executor := newTestExecutor()
	engage := NewEngage("missing", "prompt", executor)
	thought := newTestThought("test scan not found")

	_, err := engage.Scan(thought)
	if err == nil {
		t.Error("expected error when key not found")
	}
}

func TestEngageNoteMetadata(t *testing.T) {
	provider := &mockEngageProvider{
		responses: []*zyn.ProviderResponse{
			toolResponse(tc("call_abc", "search", `{"query":"meta"}`)),
			textResponse("Done"),
		},
	}
	SetProvider(provider)
	defer SetProvider(nil)

	executor := newTestExecutor()
	engage := NewEngage("answer", "Answer", executor)

	thought := newTestThought("test note metadata")
	thought.SetContent(context.Background(), "question", "Check metadata", "user")

	result, err := engage.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check tool call note
	callNote, ok := result.GetNote("answer.call.call_abc")
	if !ok {
		t.Fatal("expected tool call note")
	}
	if callNote.Metadata["type"] != "tool_call" {
		t.Errorf("expected type 'tool_call', got %q", callNote.Metadata["type"])
	}
	if callNote.Metadata["tool"] != "search" {
		t.Errorf("expected tool 'search', got %q", callNote.Metadata["tool"])
	}
	if callNote.Metadata["call_id"] != "call_abc" {
		t.Errorf("expected call_id 'call_abc', got %q", callNote.Metadata["call_id"])
	}

	// Check tool result note
	resultNote, ok := result.GetNote("answer.result.call_abc")
	if !ok {
		t.Fatal("expected tool result note")
	}
	if resultNote.Metadata["type"] != "tool_result" {
		t.Errorf("expected type 'tool_result', got %q", resultNote.Metadata["type"])
	}
	if resultNote.Source != "search" {
		t.Errorf("expected source 'search', got %q", resultNote.Source)
	}
}

func TestEngageExecutorPanicRecovery(t *testing.T) {
	provider := &mockEngageProvider{
		responses: []*zyn.ProviderResponse{
			toolResponse(tc("call_1", "search", `{"query":"panic"}`)),
			textResponse("Handled the panic"),
		},
	}
	SetProvider(provider)
	defer SetProvider(nil)

	executor := &mockToolExecutor{
		tools:   []zyn.Tool{{Name: "search", Description: "Search"}},
		results: map[string]string{},
	}
	// Override Execute to panic
	panicExecutor := &panicToolExecutor{tools: executor.tools}

	engage := NewEngage("answer", "Answer", panicExecutor)

	thought := newTestThought("test executor panic")
	thought.SetContent(context.Background(), "question", "Panic test", "user")

	result, err := engage.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error — panic should be recovered: %v", err)
	}

	output, err := engage.Scan(result)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	if len(output.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(output.ToolCalls))
	}
	if !output.ToolCalls[0].Error {
		t.Error("expected tool call to be marked as error")
	}
	if !strings.Contains(output.ToolCalls[0].Output, "panic") {
		t.Errorf("expected panic in error output, got %q", output.ToolCalls[0].Output)
	}
}

// panicToolExecutor panics on Execute.
type panicToolExecutor struct {
	tools []zyn.Tool
}

func (p *panicToolExecutor) ListTools() []zyn.Tool {
	return p.tools
}

func (p *panicToolExecutor) Execute(_ context.Context, _ zyn.ToolCall) (string, error) {
	panic("executor exploded")
}

func TestEngageContextCancellation(t *testing.T) {
	provider := &mockEngageProvider{
		responses: []*zyn.ProviderResponse{
			toolResponse(tc("call_1", "search", `{"query":"test"}`)),
			textResponse("Should not reach here"),
		},
	}
	SetProvider(provider)
	defer SetProvider(nil)

	executor := newTestExecutor()
	engage := NewEngage("answer", "Answer", executor)

	ctx, cancel := context.WithCancel(context.Background())

	thought := newTestThought("test ctx cancel")
	thought.SetContent(ctx, "question", "Cancel me", "user")

	// Cancel context before process — first iteration will fail ctx check on second iteration
	// Actually we need to cancel after the first call returns tool_use
	// So let's just cancel before Process starts
	cancel()

	_, err := engage.Process(ctx, thought)
	if err == nil {
		t.Fatal("expected error on canceled context")
	}
	if !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("expected context canceled error, got: %v", err)
	}
}

func TestEngageMaxIterationsMinimum(t *testing.T) {
	executor := newTestExecutor()
	engage := NewEngage("answer", "Answer", executor).WithMaxIterations(0)
	if engage.maxIterations != 1 {
		t.Errorf("expected maxIterations to be clamped to 1, got %d", engage.maxIterations)
	}
}
