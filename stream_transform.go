package cogito

import (
	"context"
	"fmt"
	"time"

	"github.com/zoobz-io/capitan"
	"github.com/zoobz-io/pipz"
	"github.com/zoobz-io/zyn"
)

// StreamTransform is a streaming text transformation primitive that implements
// pipz.Chainable[*Thought]. It streams LLM tokens via a callback as they arrive
// from the provider, while storing the complete accumulated response as a Note
// after streaming finishes.
//
// This enables interactive applications (chat, live analysis) to show output
// incrementally without bypassing cogito's pipeline model. Internal reasoning
// steps (Decide, Categorize, Analyze) run non-streaming as usual, and the
// final user-facing step streams tokens through the callback.
//
// If the callback is nil, StreamTransform falls back to non-streaming behavior
// via FireWithInput — the result is identical, no chunks are delivered.
//
// If the resolved provider does not implement zyn.StreamingProvider, the zyn
// layer falls back to non-streaming transparently. The callback is not invoked
// and the full response is returned after generation completes.
//
// # Retry Behavior
//
// If StreamTransform is wrapped in pipz.Retry, the callback fires from scratch
// on each retry attempt. Callers that accumulate chunks should reset their
// buffer when a new stream begins.
//
// # Introspection
//
// When enabled via WithIntrospection, a Transform synapse generates a semantic
// summary after streaming completes. The summary is stored at {key}_summary
// (or a custom key via WithSummaryKey).
type StreamTransform struct {
	identity                 pipz.Identity
	key                      string
	prompt                   string
	callback                 zyn.StreamCallback
	summaryKey               string
	useIntrospection         bool
	reasoningTemperature     float32
	introspectionTemperature float32
	provider                 Provider
	temperature              float32
}

// NewStreamTransform creates a new streaming text transformation primitive.
//
// The callback receives token chunks as they arrive from the LLM provider.
// Pass nil for non-streaming behavior (equivalent to a plain Transform call).
//
// Output Notes:
//   - {key}: Complete accumulated LLM response text
//   - {key}_summary: Semantic summary for next steps (if introspection enabled)
//
// Example:
//
//	stream := cogito.NewStreamTransform("response", "Write a detailed analysis", func(chunk string) {
//	    fmt.Print(chunk) // print tokens as they arrive
//	})
//	result, _ := stream.Process(ctx, thought)
//	content, _ := stream.Scan(result)
func NewStreamTransform(key, prompt string, callback zyn.StreamCallback) *StreamTransform {
	return &StreamTransform{
		identity:         pipz.NewIdentity(key, "Streaming transform primitive"),
		key:              key,
		prompt:           prompt,
		callback:         callback,
		useIntrospection: DefaultIntrospection,
		temperature:      DefaultReasoningTemperature,
	}
}

// Process implements pipz.Chainable[*Thought].
func (s *StreamTransform) Process(ctx context.Context, t *Thought) (*Thought, error) {
	start := time.Now()

	// Resolve provider
	provider, err := ResolveProvider(ctx, s.provider)
	if err != nil {
		return t, fmt.Errorf("stream_transform: %w", err)
	}

	// Gather unpublished notes for context
	unpublished := t.GetUnpublishedNotes()
	noteContext := RenderNotesToContext(unpublished)

	// Emit step started
	capitan.Emit(ctx, StepStarted,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(s.key),
		FieldStepType.Field("stream_transform"),
		FieldUnpublishedCount.Field(len(unpublished)),
		FieldTemperature.Field(s.temperature),
	)

	// Create transform synapse
	transformSynapse, err := zyn.Transform(s.prompt, provider)
	if err != nil {
		s.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("stream_transform: failed to create transform synapse: %w", err)
	}

	// Determine reasoning temperature
	temp := s.temperature
	if s.reasoningTemperature != 0 {
		temp = s.reasoningTemperature
	}

	input := zyn.TransformInput{
		Text:        noteContext,
		Context:     t.Intent,
		Style:       s.prompt,
		Temperature: temp,
	}

	// Execute: streaming or non-streaming based on callback
	var result string
	if s.callback != nil {
		result, err = transformSynapse.FireStreamWithInput(ctx, t.Session, input, s.callback)
		if err != nil {
			s.emitFailed(ctx, t, start, err)
			return t, fmt.Errorf("stream_transform: streaming execution failed: %w", err)
		}

		// Emit stream completed
		capitan.Emit(ctx, StreamCompleted,
			FieldTraceID.Field(t.TraceID),
			FieldStepName.Field(s.key),
			FieldStreamedSize.Field(len(result)),
		)
	} else {
		result, err = transformSynapse.FireWithInput(ctx, t.Session, input)
		if err != nil {
			s.emitFailed(ctx, t, start, err)
			return t, fmt.Errorf("stream_transform: execution failed: %w", err)
		}
	}

	// Store complete result as note
	if err := t.SetContent(ctx, s.key, result, "stream_transform"); err != nil {
		s.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("stream_transform: failed to persist note: %w", err)
	}

	// Run introspection if enabled
	if s.useIntrospection {
		if introErr := runIntrospection(ctx, t, provider, zyn.TransformInput{
			Text:    fmt.Sprintf("Transformed content:\n%s", result),
			Context: noteContext,
			Style:   "Synthesize this transformed content into context for the next reasoning step",
		}, introspectionConfig{
			stepType:                 "stream_transform",
			key:                      s.key,
			summaryKey:               s.summaryKey,
			introspectionTemperature: s.introspectionTemperature,
			synapsePrompt:            "Synthesize transformed content into context for next reasoning step",
		}); introErr != nil {
			s.emitFailed(ctx, t, start, introErr)
			return t, introErr
		}
	}

	// Mark notes as published
	t.MarkNotesPublished(ctx)

	// Emit step completed
	capitan.Emit(ctx, StepCompleted,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(s.key),
		FieldStepType.Field("stream_transform"),
		FieldStepDuration.Field(time.Since(start)),
		FieldContentSize.Field(len(result)),
	)

	return t, nil
}

// emitFailed emits a step failed event.
func (s *StreamTransform) emitFailed(ctx context.Context, t *Thought, start time.Time, err error) {
	capitan.Error(ctx, StepFailed,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(s.key),
		FieldStepType.Field("stream_transform"),
		FieldStepDuration.Field(time.Since(start)),
		FieldError.Field(err),
	)
}

// Scan retrieves the complete streamed content from a thought.
func (s *StreamTransform) Scan(t *Thought) (string, error) {
	return t.GetContent(s.key)
}

// Identity implements pipz.Chainable[*Thought].
func (s *StreamTransform) Identity() pipz.Identity {
	return s.identity
}

// Schema implements pipz.Chainable[*Thought].
func (s *StreamTransform) Schema() pipz.Node {
	return pipz.Node{Identity: s.identity, Type: "stream_transform"}
}

// Close implements pipz.Chainable[*Thought].
func (s *StreamTransform) Close() error {
	return nil
}

// Builder methods

// WithProvider sets the provider for this step.
func (s *StreamTransform) WithProvider(p Provider) *StreamTransform {
	s.provider = p
	return s
}

// WithTemperature sets the default temperature for the transform call.
func (s *StreamTransform) WithTemperature(temp float32) *StreamTransform {
	s.temperature = temp
	return s
}

// WithIntrospection enables semantic summary generation after streaming completes.
func (s *StreamTransform) WithIntrospection() *StreamTransform {
	s.useIntrospection = true
	return s
}

// WithSummaryKey sets a custom key for the introspection summary note.
// Defaults to {key}_summary if not set.
func (s *StreamTransform) WithSummaryKey(key string) *StreamTransform {
	s.summaryKey = key
	return s
}

// WithReasoningTemperature sets the temperature for the main transform call.
func (s *StreamTransform) WithReasoningTemperature(temp float32) *StreamTransform {
	s.reasoningTemperature = temp
	return s
}

// WithIntrospectionTemperature sets the temperature for the introspection summary.
func (s *StreamTransform) WithIntrospectionTemperature(temp float32) *StreamTransform {
	s.introspectionTemperature = temp
	return s
}
