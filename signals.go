package cogito

import "github.com/zoobz-io/capitan"

// Signal definitions for cogito reasoning chain events.
// Signals follow the pattern: cogito.<entity>.<event>.
var (
	// Thought lifecycle signals.
	ThoughtCreated = capitan.NewSignal(
		"cogito.thought.created",
		"New thought chain initiated with intent and trace ID",
	)

	// Step execution signals.
	StepStarted = capitan.NewSignal(
		"cogito.step.started",
		"Reasoning step began execution",
	)
	StepCompleted = capitan.NewSignal(
		"cogito.step.completed",
		"Reasoning step finished successfully",
	)
	StepFailed = capitan.NewSignal(
		"cogito.step.failed",
		"Reasoning step encountered an error",
	)

	// Note management signals.
	NoteAdded = capitan.NewSignal(
		"cogito.note.added",
		"New note added to thought context",
	)
	NotesPublished = capitan.NewSignal(
		"cogito.notes.published",
		"Notes marked as published to LLM",
	)

	// Introspection signals.
	IntrospectionCompleted = capitan.NewSignal(
		"cogito.introspection.completed",
		"Transform synapse completed semantic summary",
	)

	// Sift signals.
	SiftDecided = capitan.NewSignal(
		"cogito.sift.decided",
		"Semantic gate decision made",
	)

	// Amplify signals.
	AmplifyIterationCompleted = capitan.NewSignal(
		"cogito.amplify.iteration.completed",
		"Refinement iteration finished",
	)
	AmplifyCompleted = capitan.NewSignal(
		"cogito.amplify.completed",
		"Refinement met completion criteria",
	)

	// Stream signals.
	StreamCompleted = capitan.NewSignal(
		"cogito.stream.completed",
		"Streaming transform finished token delivery",
	)

	// Engage signals.
	EngageIterationCompleted = capitan.NewSignal(
		"cogito.engage.iteration.completed",
		"Tool use iteration completed",
	)
	EngageToolCallCompleted = capitan.NewSignal(
		"cogito.engage.tool_call.completed",
		"Individual tool call executed",
	)

	// Converge signals.
	ConvergeBranchStarted = capitan.NewSignal(
		"cogito.converge.branch.started",
		"Parallel branch began execution",
	)
	ConvergeBranchCompleted = capitan.NewSignal(
		"cogito.converge.branch.completed",
		"Parallel branch finished execution",
	)
	ConvergeSynthesisStarted = capitan.NewSignal(
		"cogito.converge.synthesis.started",
		"Synthesis phase began",
	)

)

// Field keys for cogito event data.
var (
	// Thought metadata.
	FieldIntent    = capitan.NewStringKey("intent")
	FieldTraceID   = capitan.NewStringKey("trace_id")
	FieldNoteCount = capitan.NewIntKey("note_count")

	// Step metadata.
	FieldStepName    = capitan.NewStringKey("step_name")
	FieldStepType    = capitan.NewStringKey("step_type") // decide, classify, analyze, sentiment, rank
	FieldTemperature = capitan.NewFloat32Key("temperature")
	FieldProvider    = capitan.NewStringKey("provider")

	// Note metadata.
	FieldNoteKey     = capitan.NewStringKey("note_key")
	FieldNoteSource  = capitan.NewStringKey("note_source")
	FieldContentSize = capitan.NewIntKey("content_size") // character count

	// Context metrics.
	FieldUnpublishedCount = capitan.NewIntKey("unpublished_count")
	FieldPublishedCount   = capitan.NewIntKey("published_count")
	FieldContextSize      = capitan.NewIntKey("context_size") // character count

	// Timing.
	FieldStepDuration = capitan.NewDurationKey("step_duration")

	// Error information.
	FieldError = capitan.NewErrorKey("error")

	// Decision metadata (for Sift, Amplify).
	FieldDecision   = capitan.NewBoolKey("decision")
	FieldConfidence = capitan.NewFloat64Key("confidence")

	// Iteration metadata (for Amplify).
	FieldIterationCount = capitan.NewIntKey("iteration_count")

	// Streaming metadata (for StreamTransform).
	FieldStreamedSize = capitan.NewIntKey("streamed_size") // total streamed character count

	// Tool metadata (for Engage).
	FieldToolName      = capitan.NewStringKey("tool_name")
	FieldToolCallCount = capitan.NewIntKey("tool_call_count")
	FieldToolError     = capitan.NewBoolKey("tool_error")

	// Branch metadata (for Converge).
	FieldBranchCount = capitan.NewIntKey("branch_count")
	FieldBranchName  = capitan.NewStringKey("branch_name")

)
