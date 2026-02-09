package cogito

import (
	"context"
	"fmt"
	"time"

	"github.com/zoobzio/capitan"
	"github.com/zoobzio/pipz"
)

// Relate finds resources with the same schema as a given URI.
// This enables discovering alternative data sources (caches, replicas, etc.)
// that contain the same type of data.
type Relate struct {
	identity pipz.Identity
	key      string  // Note key for result
	uri      string  // URI to find related resources for
	catalog  Catalog
}

// NewRelate creates a relate primitive that finds resources sharing the same schema.
//
// Example:
//
//	relate := cogito.NewRelate("user_sources", "db://users")
//	result, _ := relate.Process(ctx, thought)
//	// Might discover: kv://user_cache, idx://user_embeddings
func NewRelate(key, uri string) *Relate {
	return &Relate{
		identity: pipz.NewIdentity(key, "Find related catalog resources"),
		key:      key,
		uri:      uri,
	}
}

// WithCatalog sets a specific catalog for this step.
func (r *Relate) WithCatalog(c Catalog) *Relate {
	r.catalog = c
	return r
}

// Process finds related resources and stores them as a note.
func (r *Relate) Process(ctx context.Context, t *Thought) (*Thought, error) {
	start := time.Now()

	catalog, err := ResolveCatalog(ctx, r.catalog)
	if err != nil {
		r.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("relate: %w", err)
	}

	capitan.Emit(ctx, StepStarted,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(r.key),
		FieldStepType.Field("relate"),
	)

	// Find related resources
	resources := catalog.Related(r.uri)

	// Render to text
	content := RenderResourcesToContext(resources)

	// Store as note with metadata
	if err := t.SetNote(ctx, r.key, content, "relate", map[string]string{
		"source_uri":     r.uri,
		"related_count":  fmt.Sprintf("%d", len(resources)),
	}); err != nil {
		r.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("relate: failed to store note: %w", err)
	}

	capitan.Emit(ctx, StepCompleted,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(r.key),
		FieldStepType.Field("relate"),
		FieldStepDuration.Field(time.Since(start)),
	)

	return t, nil
}

// Identity implements pipz.Chainable[*Thought].
func (r *Relate) Identity() pipz.Identity {
	return r.identity
}

// Schema implements pipz.Chainable[*Thought].
func (r *Relate) Schema() pipz.Node {
	return pipz.Node{Identity: r.identity, Type: "relate"}
}

// Close implements pipz.Chainable[*Thought].
func (r *Relate) Close() error {
	return nil
}

func (r *Relate) emitFailed(ctx context.Context, t *Thought, start time.Time, err error) {
	capitan.Error(ctx, StepFailed,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(r.key),
		FieldStepType.Field("relate"),
		FieldStepDuration.Field(time.Since(start)),
		FieldError.Field(err),
	)
}

var _ pipz.Chainable[*Thought] = (*Relate)(nil)
