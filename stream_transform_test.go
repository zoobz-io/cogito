package cogito

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/zoobz-io/zyn"
)

// mockStreamTransformProvider implements both Provider and zyn.StreamingProvider.
type mockStreamTransformProvider struct {
	callCount int
	chunks    []string // chunks to deliver via Stream
}

func (m *mockStreamTransformProvider) Call(_ context.Context, messages []zyn.Message, _ float32) (*zyn.ProviderResponse, error) {
	m.callCount++

	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages provided")
	}

	lastMessage := messages[len(messages)-1]

	// Transform synapse call (introspection) — check first since it's more specific
	if strings.Contains(lastMessage.Content, "Transform:") &&
		strings.Contains(lastMessage.Content, "Synthesize") {
		return &zyn.ProviderResponse{
			Content: `{"output": "Summary of streamed content", "confidence": 0.9, "changes": ["summarized"], "reasoning": ["synthesis"]}`,
			Usage:   zyn.TokenUsage{Prompt: 10, Completion: 15, Total: 25},
		}, nil
	}

	// Primary Transform synapse call
	if strings.Contains(lastMessage.Content, "Transform:") {
		return &zyn.ProviderResponse{
			Content: `{"output": "The streamed response content.", "confidence": 0.95, "changes": ["transformed"], "reasoning": ["applied prompt"]}`,
			Usage:   zyn.TokenUsage{Prompt: 20, Completion: 30, Total: 50},
		}, nil
	}

	return &zyn.ProviderResponse{
		Content: `{"output": "fallback", "confidence": 0.5, "changes": [], "reasoning": []}`,
		Usage:   zyn.TokenUsage{Prompt: 5, Completion: 5, Total: 10},
	}, nil
}

func (m *mockStreamTransformProvider) Name() string {
	return "mock-stream-transform"
}

func (m *mockStreamTransformProvider) Stream(_ context.Context, messages []zyn.Message, temperature float32, callback zyn.StreamCallback) (*zyn.ProviderResponse, error) {
	// Get the full response via Call
	resp, err := m.Call(context.Background(), messages, temperature)
	if err != nil {
		return nil, err
	}

	// Deliver chunks via callback
	if callback != nil && len(m.chunks) > 0 {
		for _, chunk := range m.chunks {
			callback(chunk)
		}
	} else if callback != nil {
		// Default: deliver as single chunk
		callback(resp.Content)
	}

	return resp, nil
}

// mockNonStreamingProvider implements only Provider (not StreamingProvider).
type mockNonStreamingProvider struct {
	callCount int
}

func (m *mockNonStreamingProvider) Call(_ context.Context, messages []zyn.Message, _ float32) (*zyn.ProviderResponse, error) {
	m.callCount++

	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages provided")
	}

	lastMessage := messages[len(messages)-1]

	// Introspection
	if strings.Contains(lastMessage.Content, "Transform:") &&
		strings.Contains(lastMessage.Content, "Synthesize") {
		return &zyn.ProviderResponse{
			Content: `{"output": "Summary", "confidence": 0.9, "changes": ["summarized"], "reasoning": ["synthesis"]}`,
			Usage:   zyn.TokenUsage{Prompt: 10, Completion: 15, Total: 25},
		}, nil
	}

	// Primary transform
	return &zyn.ProviderResponse{
		Content: `{"output": "Non-streaming response.", "confidence": 0.9, "changes": ["transformed"], "reasoning": ["applied"]}`,
		Usage:   zyn.TokenUsage{Prompt: 15, Completion: 20, Total: 35},
	}, nil
}

func (m *mockNonStreamingProvider) Name() string {
	return "mock-non-streaming"
}

func TestStreamTransformBasic(t *testing.T) {
	provider := &mockStreamTransformProvider{
		chunks: []string{"The ", "streamed ", "response ", "content."},
	}
	SetProvider(provider)
	defer SetProvider(nil)

	var received []string
	stream := NewStreamTransform("output", "Generate analysis", func(chunk string) {
		received = append(received, chunk)
	})

	thought := newTestThought("test streaming")
	thought.SetContent(context.Background(), "input", "Some input data", "test")

	result, err := stream.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify chunks were received
	if len(received) != 4 {
		t.Errorf("expected 4 chunks, got %d", len(received))
	}

	// Verify complete content stored as note
	content, err := stream.Scan(result)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}
	if content == "" {
		t.Error("expected non-empty content")
	}
}

func TestStreamTransformNilCallback(t *testing.T) {
	provider := &mockNonStreamingProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	stream := NewStreamTransform("output", "Generate analysis", nil)

	thought := newTestThought("test nil callback")
	thought.SetContent(context.Background(), "input", "Some input data", "test")

	result, err := stream.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify content stored despite nil callback
	content, err := stream.Scan(result)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}
	if content == "" {
		t.Error("expected non-empty content")
	}

	// Verify provider was called once (no streaming path)
	if provider.callCount != 1 {
		t.Errorf("expected 1 provider call, got %d", provider.callCount)
	}
}

func TestStreamTransformScan(t *testing.T) {
	provider := &mockStreamTransformProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	stream := NewStreamTransform("my_output", "Transform text", nil)

	thought := newTestThought("test scan")
	thought.SetContent(context.Background(), "input", "data", "test")

	result, err := stream.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := stream.Scan(result)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	if content != "The streamed response content." {
		t.Errorf("expected 'The streamed response content.', got %q", content)
	}
}

func TestStreamTransformScanNotFound(t *testing.T) {
	stream := NewStreamTransform("missing", "prompt", nil)
	thought := newTestThought("test scan not found")

	_, err := stream.Scan(thought)
	if err == nil {
		t.Error("expected error when key not found")
	}
}

func TestStreamTransformWithIntrospection(t *testing.T) {
	provider := &mockStreamTransformProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	stream := NewStreamTransform("output", "Generate analysis", nil).
		WithIntrospection()

	thought := newTestThought("test introspection")
	thought.SetContent(context.Background(), "input", "data", "test")

	result, err := stream.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify summary note created
	summary, err := result.GetContent("output_summary")
	if err != nil {
		t.Fatalf("expected summary note: %v", err)
	}
	if summary == "" {
		t.Error("expected non-empty summary")
	}

	// Verify provider called twice (transform + introspection)
	if provider.callCount != 2 {
		t.Errorf("expected 2 provider calls, got %d", provider.callCount)
	}
}

func TestStreamTransformWithSummaryKey(t *testing.T) {
	provider := &mockStreamTransformProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	stream := NewStreamTransform("output", "Generate analysis", nil).
		WithIntrospection().
		WithSummaryKey("custom_summary")

	thought := newTestThought("test custom summary key")
	thought.SetContent(context.Background(), "input", "data", "test")

	result, err := stream.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify summary at custom key
	summary, err := result.GetContent("custom_summary")
	if err != nil {
		t.Fatalf("expected summary at custom key: %v", err)
	}
	if summary == "" {
		t.Error("expected non-empty summary")
	}
}

func TestStreamTransformProviderResolution(t *testing.T) {
	globalProvider := &mockNonStreamingProvider{}
	SetProvider(globalProvider)
	defer SetProvider(nil)

	stepProvider := &mockStreamTransformProvider{}

	stream := NewStreamTransform("output", "Generate analysis", nil).
		WithProvider(stepProvider)

	thought := newTestThought("test provider resolution")
	thought.SetContent(context.Background(), "input", "data", "test")

	_, err := stream.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Step provider should be used, not global
	if stepProvider.callCount != 1 {
		t.Errorf("expected step provider to be called once, got %d", stepProvider.callCount)
	}
	if globalProvider.callCount != 0 {
		t.Errorf("expected global provider not to be called, got %d", globalProvider.callCount)
	}
}

func TestStreamTransformNotesPublished(t *testing.T) {
	provider := &mockStreamTransformProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	stream := NewStreamTransform("output", "Generate analysis", nil)

	thought := newTestThought("test notes published")
	thought.SetContent(context.Background(), "input", "data", "test")

	result, err := stream.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// After Process, unpublished notes should be empty
	unpublished := result.GetUnpublishedNotes()
	if len(unpublished) != 0 {
		t.Errorf("expected 0 unpublished notes, got %d", len(unpublished))
	}
}

func TestStreamTransformBuilderMethods(t *testing.T) {
	provider := &mockStreamTransformProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	stream := NewStreamTransform("output", "Generate analysis", nil).
		WithTemperature(0.5).
		WithReasoningTemperature(0.3).
		WithIntrospectionTemperature(0.7).
		WithIntrospection().
		WithSummaryKey("summary").
		WithProvider(provider)

	thought := newTestThought("test builder methods")
	thought.SetContent(context.Background(), "input", "data", "test")

	_, err := stream.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStreamTransformNoProvider(t *testing.T) {
	SetProvider(nil)

	stream := NewStreamTransform("output", "Generate analysis", nil)
	thought := newTestThought("test no provider")

	_, err := stream.Process(context.Background(), thought)
	if err == nil {
		t.Error("expected error when no provider configured")
	}
	if !strings.Contains(err.Error(), "no provider") {
		t.Errorf("expected no provider error, got: %v", err)
	}
}

func TestStreamTransformDefaultNoIntrospection(t *testing.T) {
	provider := &mockStreamTransformProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	stream := NewStreamTransform("output", "Generate analysis", nil)

	thought := newTestThought("test default no introspection")
	thought.SetContent(context.Background(), "input", "data", "test")

	_, err := stream.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Provider should be called once (no introspection)
	if provider.callCount != 1 {
		t.Errorf("expected 1 provider call (no introspection), got %d", provider.callCount)
	}
}

func TestStreamTransformIdentity(t *testing.T) {
	stream := NewStreamTransform("my_stream", "prompt", nil)
	if stream.Identity().Name() != "my_stream" {
		t.Errorf("expected name 'my_stream', got %q", stream.Identity().Name())
	}
}

func TestStreamTransformSchema(t *testing.T) {
	stream := NewStreamTransform("my_stream", "prompt", nil)
	schema := stream.Schema()
	if schema.Type != "stream_transform" {
		t.Errorf("expected type 'stream_transform', got %q", schema.Type)
	}
}

func TestStreamTransformClose(t *testing.T) {
	stream := NewStreamTransform("my_stream", "prompt", nil)
	if err := stream.Close(); err != nil {
		t.Errorf("unexpected error on close: %v", err)
	}
}

// --- Error path tests ---

// mockStreamFailingProvider can be configured to fail at specific call counts.
type mockStreamFailingProvider struct {
	callCount int
	failAt    int // fail on this call number (1-indexed); 0 means never fail
}

func (m *mockStreamFailingProvider) Call(_ context.Context, messages []zyn.Message, _ float32) (*zyn.ProviderResponse, error) {
	m.callCount++
	if m.failAt > 0 && m.callCount >= m.failAt {
		return nil, fmt.Errorf("provider error at call %d", m.callCount)
	}

	lastMessage := messages[len(messages)-1]

	// Introspection
	if strings.Contains(lastMessage.Content, "Transform:") &&
		strings.Contains(lastMessage.Content, "Synthesize") {
		return &zyn.ProviderResponse{
			Content: `{"output": "Summary", "confidence": 0.9, "changes": ["summarized"], "reasoning": ["synthesis"]}`,
			Usage:   zyn.TokenUsage{Prompt: 10, Completion: 15, Total: 25},
		}, nil
	}

	return &zyn.ProviderResponse{
		Content: `{"output": "Response.", "confidence": 0.9, "changes": ["done"], "reasoning": ["ok"]}`,
		Usage:   zyn.TokenUsage{Prompt: 10, Completion: 10, Total: 20},
	}, nil
}

func (m *mockStreamFailingProvider) Name() string { return "mock-failing" }

// mockFailingStreamingProvider adds Stream support to mockStreamFailingProvider.
type mockFailingStreamingProvider struct {
	mockStreamFailingProvider
	failStream bool // if true, fail during Stream instead of Call
}

func (m *mockFailingStreamingProvider) Stream(_ context.Context, messages []zyn.Message, temperature float32, callback zyn.StreamCallback) (*zyn.ProviderResponse, error) {
	if m.failStream {
		m.callCount++
		return nil, fmt.Errorf("stream error")
	}
	resp, err := m.Call(context.Background(), messages, temperature)
	if err != nil {
		return nil, err
	}
	if callback != nil {
		callback(resp.Content)
	}
	return resp, nil
}

func TestStreamTransformStreamingExecutionFailure(t *testing.T) {
	provider := &mockFailingStreamingProvider{
		failStream: true,
	}
	SetProvider(provider)
	defer SetProvider(nil)

	stream := NewStreamTransform("output", "Generate analysis", func(_ string) {})

	thought := newTestThought("test streaming failure")
	thought.SetContent(context.Background(), "input", "data", "test")

	_, err := stream.Process(context.Background(), thought)
	if err == nil {
		t.Fatal("expected error on streaming failure")
	}
	if !strings.Contains(err.Error(), "streaming execution failed") {
		t.Errorf("expected streaming execution error, got: %v", err)
	}
}

func TestStreamTransformNonStreamingExecutionFailure(t *testing.T) {
	provider := &mockStreamFailingProvider{failAt: 1}
	SetProvider(provider)
	defer SetProvider(nil)

	stream := NewStreamTransform("output", "Generate analysis", nil)

	thought := newTestThought("test non-streaming failure")
	thought.SetContent(context.Background(), "input", "data", "test")

	_, err := stream.Process(context.Background(), thought)
	if err == nil {
		t.Fatal("expected error on non-streaming failure")
	}
	if !strings.Contains(err.Error(), "execution failed") {
		t.Errorf("expected execution error, got: %v", err)
	}
}

func TestStreamTransformIntrospectionFailure(t *testing.T) {
	// First call succeeds (transform), second call fails (introspection)
	provider := &mockStreamFailingProvider{failAt: 2}
	SetProvider(provider)
	defer SetProvider(nil)

	stream := NewStreamTransform("output", "Generate analysis", nil).
		WithIntrospection()

	thought := newTestThought("test introspection failure")
	thought.SetContent(context.Background(), "input", "data", "test")

	_, err := stream.Process(context.Background(), thought)
	if err == nil {
		t.Fatal("expected error on introspection failure")
	}
}

func TestStreamTransformCallbackPanicRecovery(t *testing.T) {
	provider := &mockStreamTransformProvider{
		chunks: []string{"chunk1", "chunk2"},
	}
	SetProvider(provider)
	defer SetProvider(nil)

	callCount := 0
	stream := NewStreamTransform("output", "Generate analysis", func(chunk string) {
		callCount++
		if callCount == 1 {
			panic("callback panic")
		}
	})

	thought := newTestThought("test panic recovery")
	thought.SetContent(context.Background(), "input", "data", "test")

	// Should not panic — the wrapper recovers
	result, err := stream.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Content should still be stored despite the panic
	content, err := stream.Scan(result)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}
	if content == "" {
		t.Error("expected content despite callback panic")
	}
}
