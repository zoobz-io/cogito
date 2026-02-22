//go:build testing

// Package cogitotest provides test utilities for cogito.
package cogitotest

import (
	"context"
	"testing"

	"github.com/zoobzio/cogito"
)

// NewTestThought creates a Thought for testing.
// This is a convenience function that wraps cogito.New.
func NewTestThought(t *testing.T, intent string) *cogito.Thought {
	t.Helper()
	return cogito.New(context.Background(), intent)
}

// NewTestThoughtWithTrace creates a Thought with explicit trace ID for testing.
func NewTestThoughtWithTrace(t *testing.T, intent, traceID string) *cogito.Thought {
	t.Helper()
	return cogito.NewWithTrace(context.Background(), intent, traceID)
}

// RequireContent asserts that the thought has the expected content at the given key.
func RequireContent(t *testing.T, thought *cogito.Thought, key, expected string) {
	t.Helper()
	content, err := thought.GetContent(key)
	if err != nil {
		t.Fatalf("expected content at key %q, got error: %v", key, err)
	}
	if content != expected {
		t.Fatalf("expected content %q at key %q, got %q", expected, key, content)
	}
}

// RequireNoContent asserts that the thought does not have content at the given key.
func RequireNoContent(t *testing.T, thought *cogito.Thought, key string) {
	t.Helper()
	_, err := thought.GetContent(key)
	if err == nil {
		t.Fatalf("expected no content at key %q, but found some", key)
	}
}
