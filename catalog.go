package cogito

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/zoobzio/atom"
)

// Catalog defines the interface for data catalog access.
// This is typically backed by a scio.Scio instance.
type Catalog interface {
	// Get retrieves an atom at the given URI.
	// Works with db://, kv://, and bcs:// resources.
	Get(ctx context.Context, uri string) (*atom.Atom, error)

	// Sources returns all registered resources.
	Sources() []CatalogResource

	// Databases returns all db:// resources.
	Databases() []CatalogResource

	// Stores returns all kv:// resources.
	Stores() []CatalogResource

	// Buckets returns all bcs:// resources.
	Buckets() []CatalogResource

	// Spec returns the atom spec for a specific resource.
	Spec(uri string) (atom.Spec, error)

	// FindByField returns all resources containing the given field.
	FindByField(field string) []CatalogResource

	// Related returns other resources with the same spec as the given URI.
	Related(uri string) []CatalogResource
}

// CatalogResource represents a registered data source.
type CatalogResource struct {
	URI         string
	Variant     string // "db", "kv", "bcs", "idx"
	Name        string
	Description string
	Fields      []string // Field names from spec
}

// Context key for catalog.
type catalogKeyType struct{}

var catalogKey = catalogKeyType{}

// Global catalog fallback.
var (
	globalCatalog   Catalog
	globalCatalogMu sync.RWMutex
)

// ErrNoCatalog is returned when no catalog can be resolved.
var ErrNoCatalog = errors.New("no catalog configured: set via context, step-level, or global")

// NoResourcesFound is the message returned when no resources match a query.
const NoResourcesFound = "No resources found."

// SetCatalog sets the global fallback catalog.
func SetCatalog(c Catalog) {
	globalCatalogMu.Lock()
	defer globalCatalogMu.Unlock()
	globalCatalog = c
}

// GetCatalog returns the global catalog, if set.
func GetCatalog() Catalog {
	globalCatalogMu.RLock()
	defer globalCatalogMu.RUnlock()
	return globalCatalog
}

// WithCatalog adds a catalog to the context.
func WithCatalog(ctx context.Context, c Catalog) context.Context {
	return context.WithValue(ctx, catalogKey, c)
}

// CatalogFromContext retrieves the catalog from context, if present.
func CatalogFromContext(ctx context.Context) (Catalog, bool) {
	c, ok := ctx.Value(catalogKey).(Catalog)
	return c, ok
}

// ResolveCatalog determines which catalog to use based on resolution order:
// 1. Step-level catalog (passed as argument)
// 2. Context catalog
// 3. Global catalog
// 4. Error if none found.
func ResolveCatalog(ctx context.Context, stepCatalog Catalog) (Catalog, error) {
	if stepCatalog != nil {
		return stepCatalog, nil
	}

	if c, ok := CatalogFromContext(ctx); ok {
		return c, nil
	}

	globalCatalogMu.RLock()
	c := globalCatalog
	globalCatalogMu.RUnlock()

	if c != nil {
		return c, nil
	}

	return nil, ErrNoCatalog
}

// RenderAtomToContext converts an atom to LLM-readable text.
func RenderAtomToContext(a *atom.Atom) string {
	if a == nil {
		return ""
	}

	var b strings.Builder

	for k, v := range a.Strings {
		b.WriteString(fmt.Sprintf("%s: %s\n", k, v))
	}
	for k, v := range a.Ints {
		b.WriteString(fmt.Sprintf("%s: %d\n", k, v))
	}
	for k, v := range a.Floats {
		b.WriteString(fmt.Sprintf("%s: %.2f\n", k, v))
	}
	for k, v := range a.Bools {
		b.WriteString(fmt.Sprintf("%s: %t\n", k, v))
	}
	for k, v := range a.Times {
		b.WriteString(fmt.Sprintf("%s: %s\n", k, v.Format("2006-01-02 15:04:05")))
	}

	return strings.TrimSpace(b.String())
}

// RenderResourcesToContext converts a list of resources to LLM-readable text.
func RenderResourcesToContext(resources []CatalogResource) string {
	if len(resources) == 0 {
		return NoResourcesFound
	}

	var b strings.Builder
	for i, r := range resources {
		b.WriteString(fmt.Sprintf("--- Resource %d ---\n", i+1))
		b.WriteString(fmt.Sprintf("URI: %s\n", r.URI))
		b.WriteString(fmt.Sprintf("Type: %s\n", r.Variant))
		if r.Description != "" {
			b.WriteString(fmt.Sprintf("Description: %s\n", r.Description))
		}
		if len(r.Fields) > 0 {
			b.WriteString(fmt.Sprintf("Fields: %s\n", strings.Join(r.Fields, ", ")))
		}
		b.WriteString("\n")
	}

	return strings.TrimSpace(b.String())
}
