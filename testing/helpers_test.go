//go:build testing

package cogitotest

import (
	"context"
	"testing"
)

func TestNewTestThought(t *testing.T) {
	thought := NewTestThought(t, "test intent")

	if thought == nil {
		t.Fatal("expected thought, got nil")
	}
	if thought.Intent != "test intent" {
		t.Errorf("expected intent 'test intent', got %q", thought.Intent)
	}
	if thought.ID == "" {
		t.Error("expected thought to have ID")
	}
	if thought.TraceID == "" {
		t.Error("expected thought to have TraceID")
	}
}

func TestNewTestThoughtWithTrace(t *testing.T) {
	thought := NewTestThoughtWithTrace(t, "test intent", "custom-trace-123")

	if thought == nil {
		t.Fatal("expected thought, got nil")
	}
	if thought.TraceID != "custom-trace-123" {
		t.Errorf("expected TraceID 'custom-trace-123', got %q", thought.TraceID)
	}
}

func TestRequireContent(t *testing.T) {
	ctx := context.Background()
	thought := NewTestThought(t, "test")
	_ = thought.SetContent(ctx, "key", "value", "test")

	// This should not fail.
	RequireContent(t, thought, "key", "value")
}

func TestRequireNoContent(t *testing.T) {
	thought := NewTestThought(t, "test")

	// This should not fail since "missing" key doesn't exist.
	RequireNoContent(t, thought, "missing")
}
