package cogito

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/zoobz-io/capitan"
	"github.com/zoobz-io/pipz"
	"github.com/zoobz-io/zyn"
)

// ToolExecutor provides tools to the LLM and handles their execution.
// This interface is intentionally compatible with ago.Executor — consumers
// can pass an ago executor directly without adaptation.
type ToolExecutor interface {
	// ListTools returns the set of tools available to the LLM.
	ListTools() []zyn.Tool

	// Execute runs a tool call and returns the result as a string.
	// Errors are fed back to the LLM as tool results, not as Process failures.
	Execute(ctx context.Context, call zyn.ToolCall) (string, error)
}

// EngageResult captures the outcome of a tool execution loop.
type EngageResult struct {
	Content    string       `json:"content"`    // Final text response from LLM
	Iterations int          `json:"iterations"` // Number of loop iterations (1 = no tool use)
	Completed  bool         `json:"completed"`  // Whether the LLM finished naturally (vs max iterations exhausted)
	ToolCalls  []ToolRecord `json:"tool_calls"` // All tool calls made during engagement
}

// ToolRecord captures a single tool invocation and its result.
type ToolRecord struct {
	Name   string `json:"name"`   // Tool name
	Input  string `json:"input"`  // JSON input passed to tool
	Output string `json:"output"` // Tool result or error text
	Error  bool   `json:"error"`  // Whether the tool returned an error
}

// Engage is an LLM-driven tool execution primitive that implements
// pipz.Chainable[*Thought]. It runs a loop where the LLM sees available
// tools, decides which to call, receives results, and iterates until it
// has enough information to respond with text.
//
// This is fundamentally different from server-authored primitives (Decide,
// Analyze, etc.) where the application defines the execution plan. With
// Engage, the LLM authors the execution plan at runtime.
//
// The primitive calls the provider directly via zyn.ToolProvider rather than
// through zyn synapses, because tool use requires raw message-level control
// for the call/result loop.
//
// # Tool Errors
//
// Tool execution errors are fed back to the LLM as tool results, not as
// Process failures. This lets the LLM adapt — it may retry with different
// input or use a different tool.
//
// # Streaming
//
// When a StreamEventCallback is set via WithStreamEventCallback, text tokens
// are streamed to the callback as they arrive. Tool call events are also
// delivered via the callback. The caller can filter by event type.
// If the provider does not implement zyn.ToolStreamingProvider, streaming
// falls back to non-streaming transparently.
//
// # Retry Behavior
//
// If Engage is wrapped in pipz.Retry, the entire tool loop restarts from
// scratch on each attempt — including the callback.
type Engage struct {
	identity      pipz.Identity
	key           string
	prompt        string
	executor      ToolExecutor
	maxIterations int
	callback      zyn.StreamEventCallback
	provider      Provider
	temperature   float32
}

// NewEngage creates a new tool execution primitive.
//
// The prompt frames the engagement — it becomes the user message content
// combined with accumulated note context. The executor provides the tools
// and handles their execution.
//
// Output Notes:
//   - {key}: JSON-serialized EngageResult with final content and tool audit trail
//   - {key}.call.{call_id}: Tool call input (per call)
//   - {key}.result.{call_id}: Tool call result (per call)
//
// Example:
//
//	engage := cogito.NewEngage("answer", "Answer the user's question using the available tools", executor)
//	result, _ := engage.Process(ctx, thought)
//	output, _ := engage.Scan(result)
//	fmt.Println(output.Content, "in", output.Iterations, "iterations using", len(output.ToolCalls), "tools")
func NewEngage(key, prompt string, executor ToolExecutor) *Engage {
	return &Engage{
		identity:      pipz.NewIdentity(key, "Tool execution primitive"),
		key:           key,
		prompt:        prompt,
		executor:      executor,
		maxIterations: 10,
		temperature:   DefaultReasoningTemperature,
	}
}

// Process implements pipz.Chainable[*Thought].
func (e *Engage) Process(ctx context.Context, t *Thought) (*Thought, error) {
	start := time.Now()

	// Resolve provider
	provider, err := ResolveProvider(ctx, e.provider)
	if err != nil {
		return t, fmt.Errorf("engage: %w", err)
	}

	// Type-assert to ToolProvider
	toolProvider, ok := provider.(zyn.ToolProvider)
	if !ok {
		return t, fmt.Errorf("engage: provider %s does not support tools", provider.Name())
	}

	// Get tools from executor
	tools := e.executor.ListTools()

	// Get unpublished notes for context
	unpublished := t.GetUnpublishedNotes()
	noteContext := RenderNotesToContext(unpublished)

	// Emit step started
	capitan.Emit(ctx, StepStarted,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(e.key),
		FieldStepType.Field("engage"),
		FieldUnpublishedCount.Field(len(unpublished)),
		FieldTemperature.Field(e.temperature),
		FieldToolCallCount.Field(len(tools)),
	)

	// Build initial messages from session snapshot + user message
	messages := t.Session.Messages()
	userContent := e.prompt
	if noteContext != "" {
		userContent = e.prompt + "\n\n" + noteContext
	}
	messages = append(messages, zyn.Message{
		Role:    zyn.RoleUser,
		Content: userContent,
	})

	// Check for streaming provider
	var toolStreamProvider zyn.ToolStreamingProvider
	if e.callback != nil {
		tsp, ok := provider.(zyn.ToolStreamingProvider)
		if ok {
			toolStreamProvider = tsp
		}
	}

	// Tool execution loop
	var records []ToolRecord
	var lastResp *zyn.ProviderResponse
	var completed bool
	iteration := 0

	for iteration < e.maxIterations {
		iteration++

		// Call provider
		var resp *zyn.ProviderResponse
		if toolStreamProvider != nil {
			// Wrap callback in panic recovery
			safe := func(event zyn.StreamEvent) {
				defer func() {
					if r := recover(); r != nil {
						capitan.Error(ctx, StepFailed,
							FieldTraceID.Field(t.TraceID),
							FieldStepName.Field(e.key),
							FieldStepType.Field("engage"),
							FieldError.Field(fmt.Errorf("callback panic: %v", r)),
						)
					}
				}()
				e.callback(event)
			}
			resp, err = toolStreamProvider.StreamWithTools(ctx, messages, e.temperature, tools, safe)
		} else {
			resp, err = toolProvider.CallWithTools(ctx, messages, e.temperature, tools)
		}
		if err != nil {
			e.emitFailed(ctx, t, start, err)
			return t, fmt.Errorf("engage: provider call failed at iteration %d: %w", iteration, err)
		}
		lastResp = resp

		// Terminal: text response or no tool calls
		if resp.StopReason != zyn.StopReasonToolUse || len(resp.ToolCalls) == 0 {
			completed = true
			// Append assistant message
			messages = append(messages, zyn.Message{
				Role:    zyn.RoleAssistant,
				Content: resp.Content,
			})
			break
		}

		// Tool use: append assistant message with tool calls
		messages = append(messages, zyn.Message{
			Role:      zyn.RoleAssistant,
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		// Execute each tool call
		for _, tc := range resp.ToolCalls {
			// Store tool call note
			callKey := fmt.Sprintf("%s.call.%s", e.key, tc.ID)
			if noteErr := t.SetNote(ctx, callKey, string(tc.Input), "engage", map[string]string{
				"type":      "tool_call",
				"tool":      tc.Name,
				"call_id":   tc.ID,
				"iteration": fmt.Sprint(iteration),
			}); noteErr != nil {
				e.emitFailed(ctx, t, start, noteErr)
				return t, fmt.Errorf("engage: failed to persist tool call note: %w", noteErr)
			}

			// Execute tool
			result, execErr := e.executor.Execute(ctx, tc)
			record := ToolRecord{
				Name:  tc.Name,
				Input: string(tc.Input),
			}
			if execErr != nil {
				result = execErr.Error()
				record.Error = true
			}
			record.Output = result
			records = append(records, record)

			// Store tool result note
			resultKey := fmt.Sprintf("%s.result.%s", e.key, tc.ID)
			if noteErr := t.SetNote(ctx, resultKey, result, tc.Name, map[string]string{
				"type":      "tool_result",
				"tool":      tc.Name,
				"call_id":   tc.ID,
				"iteration": fmt.Sprint(iteration),
				"error":     fmt.Sprint(execErr != nil),
			}); noteErr != nil {
				e.emitFailed(ctx, t, start, noteErr)
				return t, fmt.Errorf("engage: failed to persist tool result note: %w", noteErr)
			}

			// Append tool result message
			messages = append(messages, zyn.Message{
				Role:       zyn.RoleTool,
				Content:    result,
				ToolCallID: tc.ID,
			})

			// Emit tool call completed
			capitan.Emit(ctx, EngageToolCallCompleted,
				FieldTraceID.Field(t.TraceID),
				FieldStepName.Field(e.key),
				FieldToolName.Field(tc.Name),
				FieldToolError.Field(execErr != nil),
				FieldIterationCount.Field(iteration),
			)
		}

		// Emit iteration completed
		capitan.Emit(ctx, EngageIterationCompleted,
			FieldTraceID.Field(t.TraceID),
			FieldStepName.Field(e.key),
			FieldIterationCount.Field(iteration),
			FieldToolCallCount.Field(len(resp.ToolCalls)),
		)
	}

	// Update session with full conversation including tool messages
	t.Session.SetMessages(messages)

	// Store result
	content := ""
	if lastResp != nil {
		content = lastResp.Content
	}
	result := EngageResult{
		Content:    content,
		Iterations: iteration,
		Completed:  completed,
		ToolCalls:  records,
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		e.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("engage: failed to marshal result: %w", err)
	}
	if err := t.SetContent(ctx, e.key, string(resultJSON), "engage"); err != nil {
		e.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("engage: failed to persist result note: %w", err)
	}

	// Mark notes as published
	t.MarkNotesPublished(ctx)

	// Emit step completed
	capitan.Emit(ctx, StepCompleted,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(e.key),
		FieldStepType.Field("engage"),
		FieldStepDuration.Field(time.Since(start)),
		FieldIterationCount.Field(iteration),
		FieldToolCallCount.Field(len(records)),
	)

	return t, nil
}

// emitFailed emits a step failed event.
func (e *Engage) emitFailed(ctx context.Context, t *Thought, start time.Time, err error) {
	capitan.Error(ctx, StepFailed,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(e.key),
		FieldStepType.Field("engage"),
		FieldStepDuration.Field(time.Since(start)),
		FieldError.Field(err),
	)
}

// Scan retrieves the typed engage result from a thought.
func (e *Engage) Scan(t *Thought) (*EngageResult, error) {
	content, err := t.GetContent(e.key)
	if err != nil {
		return nil, fmt.Errorf("engage scan: %w", err)
	}
	var result EngageResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("engage scan: failed to unmarshal result: %w", err)
	}
	return &result, nil
}

// Identity implements pipz.Chainable[*Thought].
func (e *Engage) Identity() pipz.Identity {
	return e.identity
}

// Schema implements pipz.Chainable[*Thought].
func (e *Engage) Schema() pipz.Node {
	return pipz.Node{Identity: e.identity, Type: "engage"}
}

// Close implements pipz.Chainable[*Thought].
func (e *Engage) Close() error {
	return nil
}

// Builder methods

// WithProvider sets the provider for this step.
func (e *Engage) WithProvider(p Provider) *Engage {
	e.provider = p
	return e
}

// WithTemperature sets the temperature for provider calls.
func (e *Engage) WithTemperature(temp float32) *Engage {
	e.temperature = temp
	return e
}

// WithMaxIterations sets the maximum number of tool loop iterations.
// Minimum 1.
func (e *Engage) WithMaxIterations(n int) *Engage {
	if n < 1 {
		n = 1
	}
	e.maxIterations = n
	return e
}

// WithStreamEventCallback sets a callback for streaming text tokens and tool events.
// The callback receives all stream events — filter by event.Type if you only want text.
// If the provider does not implement zyn.ToolStreamingProvider, streaming falls back
// to non-streaming transparently.
func (e *Engage) WithStreamEventCallback(cb zyn.StreamEventCallback) *Engage {
	e.callback = cb
	return e
}
