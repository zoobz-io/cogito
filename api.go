// Package cogito provides LLM-powered reasoning chains for Go.
//
// cogito implements a Thought-Note architecture for building autonomous
// systems that reason and adapt.
//
// # Core Types
//
// The package is built around two core concepts:
//
//   - [Thought] - A reasoning context that accumulates information across pipeline steps
//   - [Note] - Atomic units of information (key-value pairs with metadata)
//
// # Creating Thoughts
//
// Use [New] or [NewWithTrace] to create thoughts:
//
//	thought := cogito.New(ctx, "analyze customer feedback")
//	thought.SetContent(ctx, "feedback", customerMessage, "input")
//
// # Primitives
//
// Cogito provides a comprehensive set of reasoning primitives:
//
// Decision & Analysis:
//   - [NewDecide] - Binary yes/no decisions with confidence scores
//   - [NewAnalyze] - Extract structured data into typed results
//   - [NewCategorize] - Classify into one of N categories
//   - [NewAssess] - Sentiment analysis with emotional scoring
//   - [NewPrioritize] - Rank items by specified criteria
//
// Control Flow:
//   - [NewSift] - Semantic gate - LLM decides whether to execute wrapped processor
//   - [NewDiscern] - Semantic router - LLM classifies and routes to different processors
//
// Reflection:
//   - [NewReflect] - Consolidate current Thought's Notes into a summary
//
// Session Management:
//   - [NewReset] - Clear session state
//   - [NewCompress] - LLM-summarize session history to reduce tokens
//   - [NewTruncate] - Sliding window session trimming (no LLM)
//
// Streaming:
//   - [NewStreamTransform] - Stream LLM tokens via callback, store complete result
//
// Tool Use:
//   - [NewEngage] - LLM-driven tool execution loop with streaming support
//
// Synthesis:
//   - [NewAmplify] - Iterative refinement until criteria met
//   - [NewConverge] - Parallel execution with semantic synthesis
//
// # Pipeline Helpers
//
// Cogito wraps pipz connectors for Thought processing:
//
//   - [Sequence] - Sequential execution
//   - [Filter] - Conditional execution
//   - [Switch] - Route to different processors
//   - [Fallback] - Try alternatives on failure
//   - [Retry] - Retry on failure
//   - [Backoff] - Retry with exponential backoff
//   - [Timeout] - Enforce time limits
//   - [Concurrent] - Run processors in parallel
//   - [Race] - Return first successful result
//
// # Provider
//
// LLM access uses a resolution hierarchy:
//
//  1. Explicit parameter (.WithProvider(p))
//  2. Context value (cogito.WithProvider(ctx, p))
//  3. Global default (cogito.SetProvider(p))
//
// Use [SetProvider] to configure the global default:
//
//	cogito.SetProvider(myProvider)
//
// # Observability
//
// Cogito emits capitan signals throughout execution. See [signals.go] for
// the complete list of events including ThoughtCreated, StepStarted,
// StepCompleted, StepFailed, NoteAdded, and NotesPublished.
package cogito
