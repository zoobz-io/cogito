package cogito

import (
	"context"
	"errors"
	"testing"

	"github.com/zoobzio/atom"
)

// mockCatalog implements Catalog for testing.
type mockCatalog struct {
	data      map[string]*atom.Atom
	resources []CatalogResource
	getErr    error
}

func newMockCatalog() *mockCatalog {
	return &mockCatalog{
		data:      make(map[string]*atom.Atom),
		resources: []CatalogResource{},
	}
}

func (m *mockCatalog) Get(_ context.Context, uri string) (*atom.Atom, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	a, ok := m.data[uri]
	if !ok {
		return nil, errors.New("not found")
	}
	return a, nil
}

func (m *mockCatalog) Sources() []CatalogResource {
	return m.resources
}

func (m *mockCatalog) Databases() []CatalogResource {
	var result []CatalogResource
	for _, r := range m.resources {
		if r.Variant == "db" {
			result = append(result, r)
		}
	}
	return result
}

func (m *mockCatalog) Stores() []CatalogResource {
	var result []CatalogResource
	for _, r := range m.resources {
		if r.Variant == "kv" {
			result = append(result, r)
		}
	}
	return result
}

func (m *mockCatalog) Buckets() []CatalogResource {
	var result []CatalogResource
	for _, r := range m.resources {
		if r.Variant == "bcs" {
			result = append(result, r)
		}
	}
	return result
}

func (m *mockCatalog) Spec(_ string) (atom.Spec, error) {
	return atom.Spec{}, nil
}

func (m *mockCatalog) FindByField(field string) []CatalogResource {
	var result []CatalogResource
	for _, r := range m.resources {
		for _, f := range r.Fields {
			if f == field {
				result = append(result, r)
				break
			}
		}
	}
	return result
}

func (m *mockCatalog) Related(_ string) []CatalogResource {
	// Return all resources except the first one as "related"
	if len(m.resources) <= 1 {
		return []CatalogResource{}
	}
	return m.resources[1:]
}

var _ Catalog = (*mockCatalog)(nil)

func TestSetGetCatalog(t *testing.T) {
	// Clear global catalog first
	SetCatalog(nil)

	// Should be nil initially
	if c := GetCatalog(); c != nil {
		t.Error("expected nil catalog")
	}

	// Set global catalog
	mock := newMockCatalog()
	SetCatalog(mock)

	// Should retrieve it
	c := GetCatalog()
	if c == nil {
		t.Fatal("expected catalog to be set")
	}

	// Clean up
	SetCatalog(nil)
}

func TestWithCatalog(t *testing.T) {
	mock := newMockCatalog()
	ctx := WithCatalog(context.Background(), mock)

	c, ok := CatalogFromContext(ctx)
	if !ok {
		t.Fatal("expected catalog in context")
	}

	if c != mock {
		t.Error("expected same catalog instance")
	}
}

func TestCatalogFromContextMissing(t *testing.T) {
	ctx := context.Background()

	_, ok := CatalogFromContext(ctx)
	if ok {
		t.Error("expected no catalog in context")
	}
}

func TestResolveCatalogStepLevel(t *testing.T) {
	// Set up all three levels
	SetCatalog(newMockCatalog())
	ctx := WithCatalog(context.Background(), newMockCatalog())
	stepCatalog := newMockCatalog()

	// Step-level should win
	c, err := ResolveCatalog(ctx, stepCatalog)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if c != stepCatalog {
		t.Error("expected step-level catalog")
	}

	// Clean up
	SetCatalog(nil)
}

func TestResolveCatalogContext(t *testing.T) {
	// Set global but use context
	SetCatalog(newMockCatalog())
	contextCatalog := newMockCatalog()
	ctx := WithCatalog(context.Background(), contextCatalog)

	// Context should win over global (no step-level)
	c, err := ResolveCatalog(ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if c != contextCatalog {
		t.Error("expected context catalog")
	}

	// Clean up
	SetCatalog(nil)
}

func TestResolveCatalogGlobal(t *testing.T) {
	globalCatalog := newMockCatalog()
	SetCatalog(globalCatalog)
	ctx := context.Background()

	// Global should be used (no step-level or context)
	c, err := ResolveCatalog(ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if c != globalCatalog {
		t.Error("expected global catalog")
	}

	// Clean up
	SetCatalog(nil)
}

func TestResolveCatalogNone(t *testing.T) {
	// Make sure global is cleared
	SetCatalog(nil)
	ctx := context.Background()

	// Should error when no catalog is available
	_, err := ResolveCatalog(ctx, nil)
	if err == nil {
		t.Error("expected error when no catalog is configured")
	}

	if !errors.Is(err, ErrNoCatalog) {
		t.Errorf("expected ErrNoCatalog, got %v", err)
	}
}

func TestRenderAtomToContext(t *testing.T) {
	a := &atom.Atom{
		Strings: map[string]string{"name": "Alice", "email": "alice@example.com"},
		Ints:    map[string]int64{"age": 30},
		Floats:  map[string]float64{"balance": 100.50},
		Bools:   map[string]bool{"active": true},
	}

	result := RenderAtomToContext(a)

	// Check that all fields are present
	if result == "" {
		t.Error("expected non-empty result")
	}

	// Check specific values are rendered
	tests := []string{"name: Alice", "age: 30", "balance: 100.50", "active: true"}
	for _, expected := range tests {
		found := false
		for _, line := range splitLines(result) {
			if line == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %q in result, got:\n%s", expected, result)
		}
	}
}

func TestRenderAtomToContext_Nil(t *testing.T) {
	result := RenderAtomToContext(nil)
	if result != "" {
		t.Errorf("expected empty string for nil atom, got %q", result)
	}
}

func TestRenderResourcesToContext(t *testing.T) {
	resources := []CatalogResource{
		{
			URI:         "db://users",
			Variant:     "db",
			Description: "User accounts",
			Fields:      []string{"id", "name", "email"},
		},
		{
			URI:     "kv://sessions",
			Variant: "kv",
		},
	}

	result := RenderResourcesToContext(resources)

	if result == "" {
		t.Error("expected non-empty result")
	}

	// Check that URIs are present
	if !containsString(result, "db://users") {
		t.Error("expected db://users in result")
	}
	if !containsString(result, "kv://sessions") {
		t.Error("expected kv://sessions in result")
	}
}

func TestRenderResourcesToContext_Empty(t *testing.T) {
	result := RenderResourcesToContext([]CatalogResource{})
	if result != NoResourcesFound {
		t.Errorf("expected %q, got %q", NoResourcesFound, result)
	}
}

func TestConcurrentCatalogAccess(t *testing.T) {
	mock := newMockCatalog()

	// Concurrent sets and gets
	done := make(chan bool)
	for i := 0; i < 100; i++ {
		go func() {
			SetCatalog(mock)
			_ = GetCatalog()
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 100; i++ {
		<-done
	}

	// Should not panic
	c := GetCatalog()
	if c == nil {
		t.Error("expected catalog after concurrent access")
	}

	// Clean up
	SetCatalog(nil)
}

// Helper functions

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i, c := range s {
		if c == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
