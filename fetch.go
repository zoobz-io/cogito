package cogito

import (
	"context"
	"fmt"
	"time"

	"github.com/zoobzio/capitan"
	"github.com/zoobzio/pipz"
)

// Fetch retrieves data from a catalog URI and adds it to the thought.
// It supports both static URIs and dynamic URIs constructed from thought content.
type Fetch struct {
	identity pipz.Identity
	key      string   // Note key for result
	uri      string   // Static URI or base URI for dynamic
	uriKey   string   // If set, append thought content at this key to URI
	catalog  Catalog
}

// NewFetch creates a fetch primitive with a static URI.
//
// Example:
//
//	fetch := cogito.NewFetch("user", "db://users/alice")
//	result, _ := fetch.Process(ctx, thought)
//	content, _ := result.GetContent("user")
func NewFetch(key, uri string) *Fetch {
	return &Fetch{
		identity: pipz.NewIdentity(key, "Fetch data from catalog"),
		key:      key,
		uri:      uri,
	}
}

// NewFetchDynamic creates a fetch primitive that constructs the URI from thought content.
// The final URI is baseURI + thought.GetContent(uriKey).
//
// Example:
//
//	thought.SetContent(ctx, "user_id", "alice", "input")
//	fetch := cogito.NewFetchDynamic("user", "db://users/", "user_id")
//	// Fetches from db://users/alice
func NewFetchDynamic(key, baseURI, uriKey string) *Fetch {
	return &Fetch{
		identity: pipz.NewIdentity(key, "Fetch data from catalog (dynamic)"),
		key:      key,
		uri:      baseURI,
		uriKey:   uriKey,
	}
}

// WithCatalog sets a specific catalog for this step.
func (f *Fetch) WithCatalog(c Catalog) *Fetch {
	f.catalog = c
	return f
}

// Process fetches data from the catalog and stores it as a note.
func (f *Fetch) Process(ctx context.Context, t *Thought) (*Thought, error) {
	start := time.Now()

	catalog, err := ResolveCatalog(ctx, f.catalog)
	if err != nil {
		f.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("fetch: %w", err)
	}

	// Build URI
	uri := f.uri
	if f.uriKey != "" {
		suffix, getErr := t.GetContent(f.uriKey)
		if getErr != nil {
			f.emitFailed(ctx, t, start, getErr)
			return t, fmt.Errorf("fetch: failed to get URI key %q: %w", f.uriKey, getErr)
		}
		uri = f.uri + suffix
	}

	capitan.Emit(ctx, StepStarted,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(f.key),
		FieldStepType.Field("fetch"),
	)

	// Fetch from catalog
	atom, err := catalog.Get(ctx, uri)
	if err != nil {
		f.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("fetch: failed to get %q: %w", uri, err)
	}

	// Render atom to text
	content := RenderAtomToContext(atom)

	// Store as note with metadata
	if err := t.SetNote(ctx, f.key, content, "fetch", map[string]string{
		"source_uri": uri,
	}); err != nil {
		f.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("fetch: failed to store note: %w", err)
	}

	capitan.Emit(ctx, StepCompleted,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(f.key),
		FieldStepType.Field("fetch"),
		FieldStepDuration.Field(time.Since(start)),
	)

	return t, nil
}

// Identity implements pipz.Chainable[*Thought].
func (f *Fetch) Identity() pipz.Identity {
	return f.identity
}

// Schema implements pipz.Chainable[*Thought].
func (f *Fetch) Schema() pipz.Node {
	return pipz.Node{Identity: f.identity, Type: "fetch"}
}

// Close implements pipz.Chainable[*Thought].
func (f *Fetch) Close() error {
	return nil
}

func (f *Fetch) emitFailed(ctx context.Context, t *Thought, start time.Time, err error) {
	capitan.Error(ctx, StepFailed,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(f.key),
		FieldStepType.Field("fetch"),
		FieldStepDuration.Field(time.Since(start)),
		FieldError.Field(err),
	)
}

var _ pipz.Chainable[*Thought] = (*Fetch)(nil)
