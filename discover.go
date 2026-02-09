package cogito

import (
	"context"
	"fmt"
	"time"

	"github.com/zoobzio/capitan"
	"github.com/zoobzio/pipz"
)

// Discover introspects the catalog and adds available resources to the thought.
// It can list all resources or filter by variant or field name.
type Discover struct {
	identity pipz.Identity
	key      string  // Note key for result
	variant  string  // Optional: filter by "db", "kv", "bcs", "idx"
	field    string  // Optional: filter by field name
	catalog  Catalog
}

// NewDiscover creates a discover primitive that lists all available resources.
//
// Example:
//
//	discover := cogito.NewDiscover("available_data")
//	result, _ := discover.Process(ctx, thought)
//	content, _ := result.GetContent("available_data")
func NewDiscover(key string) *Discover {
	return &Discover{
		identity: pipz.NewIdentity(key, "Discover catalog resources"),
		key:      key,
	}
}

// WithVariant filters to a specific resource variant.
// Valid variants: "db", "kv", "bcs", "idx".
func (d *Discover) WithVariant(v string) *Discover {
	d.variant = v
	return d
}

// WithField filters to resources containing a specific field.
func (d *Discover) WithField(field string) *Discover {
	d.field = field
	return d
}

// WithCatalog sets a specific catalog for this step.
func (d *Discover) WithCatalog(c Catalog) *Discover {
	d.catalog = c
	return d
}

// Process queries the catalog and stores discovered resources as a note.
func (d *Discover) Process(ctx context.Context, t *Thought) (*Thought, error) {
	start := time.Now()

	catalog, err := ResolveCatalog(ctx, d.catalog)
	if err != nil {
		d.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("discover: %w", err)
	}

	capitan.Emit(ctx, StepStarted,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(d.key),
		FieldStepType.Field("discover"),
	)

	// Get resources based on filters
	var resources []CatalogResource

	switch {
	case d.field != "":
		// Filter by field
		resources = catalog.FindByField(d.field)
	case d.variant != "":
		// Filter by variant
		switch d.variant {
		case "db":
			resources = catalog.Databases()
		case "kv":
			resources = catalog.Stores()
		case "bcs":
			resources = catalog.Buckets()
		default:
			resources = catalog.Sources()
		}
	default:
		// All resources
		resources = catalog.Sources()
	}

	// Render to text
	content := RenderResourcesToContext(resources)

	// Store as note with metadata
	metadata := map[string]string{
		"resource_count": fmt.Sprintf("%d", len(resources)),
	}
	if d.variant != "" {
		metadata["filter_variant"] = d.variant
	}
	if d.field != "" {
		metadata["filter_field"] = d.field
	}

	if err := t.SetNote(ctx, d.key, content, "discover", metadata); err != nil {
		d.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("discover: failed to store note: %w", err)
	}

	capitan.Emit(ctx, StepCompleted,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(d.key),
		FieldStepType.Field("discover"),
		FieldStepDuration.Field(time.Since(start)),
	)

	return t, nil
}

// Identity implements pipz.Chainable[*Thought].
func (d *Discover) Identity() pipz.Identity {
	return d.identity
}

// Schema implements pipz.Chainable[*Thought].
func (d *Discover) Schema() pipz.Node {
	return pipz.Node{Identity: d.identity, Type: "discover"}
}

// Close implements pipz.Chainable[*Thought].
func (d *Discover) Close() error {
	return nil
}

func (d *Discover) emitFailed(ctx context.Context, t *Thought, start time.Time, err error) {
	capitan.Error(ctx, StepFailed,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(d.key),
		FieldStepType.Field("discover"),
		FieldStepDuration.Field(time.Since(start)),
		FieldError.Field(err),
	)
}

var _ pipz.Chainable[*Thought] = (*Discover)(nil)
