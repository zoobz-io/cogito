package cogito

import (
	"context"
	"errors"
	"testing"
)

func TestDiscover_AllSources(t *testing.T) {
	ctx := context.Background()
	mem := newMockMemory()
	thought, _ := New(ctx, mem, "test")

	catalog := newMockCatalog()
	catalog.resources = []CatalogResource{
		{URI: "db://users", Variant: "db", Description: "User accounts"},
		{URI: "db://orders", Variant: "db", Description: "Order history"},
		{URI: "kv://sessions", Variant: "kv"},
	}
	ctx = WithCatalog(ctx, catalog)

	discover := NewDiscover("sources")
	result, err := discover.Process(ctx, thought)
	if err != nil {
		t.Fatalf("discover failed: %v", err)
	}

	content, err := result.GetContent("sources")
	if err != nil {
		t.Fatalf("expected content: %v", err)
	}

	// Check all resources are listed
	if !containsString(content, "db://users") {
		t.Error("expected db://users in result")
	}
	if !containsString(content, "db://orders") {
		t.Error("expected db://orders in result")
	}
	if !containsString(content, "kv://sessions") {
		t.Error("expected kv://sessions in result")
	}

	// Check metadata
	note, _ := result.GetNote("sources")
	if note.Metadata["resource_count"] != "3" {
		t.Errorf("expected resource_count '3', got %q", note.Metadata["resource_count"])
	}
}

func TestDiscover_FilterByVariant_Database(t *testing.T) {
	ctx := context.Background()
	mem := newMockMemory()
	thought, _ := New(ctx, mem, "test")

	catalog := newMockCatalog()
	catalog.resources = []CatalogResource{
		{URI: "db://users", Variant: "db"},
		{URI: "db://orders", Variant: "db"},
		{URI: "kv://sessions", Variant: "kv"},
		{URI: "bcs://uploads", Variant: "bcs"},
	}
	ctx = WithCatalog(ctx, catalog)

	discover := NewDiscover("databases").WithVariant("db")
	result, err := discover.Process(ctx, thought)
	if err != nil {
		t.Fatalf("discover failed: %v", err)
	}

	content, _ := result.GetContent("databases")

	// Should have databases
	if !containsString(content, "db://users") {
		t.Error("expected db://users in result")
	}
	if !containsString(content, "db://orders") {
		t.Error("expected db://orders in result")
	}

	// Should NOT have other variants
	if containsString(content, "kv://sessions") {
		t.Error("should not have kv://sessions")
	}
	if containsString(content, "bcs://uploads") {
		t.Error("should not have bcs://uploads")
	}

	// Check metadata
	note, _ := result.GetNote("databases")
	if note.Metadata["filter_variant"] != "db" {
		t.Errorf("expected filter_variant 'db', got %q", note.Metadata["filter_variant"])
	}
	if note.Metadata["resource_count"] != "2" {
		t.Errorf("expected resource_count '2', got %q", note.Metadata["resource_count"])
	}
}

func TestDiscover_FilterByVariant_Store(t *testing.T) {
	ctx := context.Background()
	mem := newMockMemory()
	thought, _ := New(ctx, mem, "test")

	catalog := newMockCatalog()
	catalog.resources = []CatalogResource{
		{URI: "db://users", Variant: "db"},
		{URI: "kv://sessions", Variant: "kv"},
		{URI: "kv://cache", Variant: "kv"},
	}
	ctx = WithCatalog(ctx, catalog)

	discover := NewDiscover("stores").WithVariant("kv")
	result, err := discover.Process(ctx, thought)
	if err != nil {
		t.Fatalf("discover failed: %v", err)
	}

	content, _ := result.GetContent("stores")

	if !containsString(content, "kv://sessions") {
		t.Error("expected kv://sessions in result")
	}
	if !containsString(content, "kv://cache") {
		t.Error("expected kv://cache in result")
	}
	if containsString(content, "db://users") {
		t.Error("should not have db://users")
	}
}

func TestDiscover_FilterByField(t *testing.T) {
	ctx := context.Background()
	mem := newMockMemory()
	thought, _ := New(ctx, mem, "test")

	catalog := newMockCatalog()
	catalog.resources = []CatalogResource{
		{URI: "db://users", Variant: "db", Fields: []string{"id", "email", "name"}},
		{URI: "db://orders", Variant: "db", Fields: []string{"id", "user_id", "total"}},
		{URI: "db://contacts", Variant: "db", Fields: []string{"id", "email", "phone"}},
	}
	ctx = WithCatalog(ctx, catalog)

	discover := NewDiscover("email_sources").WithField("email")
	result, err := discover.Process(ctx, thought)
	if err != nil {
		t.Fatalf("discover failed: %v", err)
	}

	content, _ := result.GetContent("email_sources")

	// Should have resources with email field
	if !containsString(content, "db://users") {
		t.Error("expected db://users in result")
	}
	if !containsString(content, "db://contacts") {
		t.Error("expected db://contacts in result")
	}

	// Should NOT have orders (no email field)
	if containsString(content, "db://orders") {
		t.Error("should not have db://orders (no email field)")
	}

	// Check metadata
	note, _ := result.GetNote("email_sources")
	if note.Metadata["filter_field"] != "email" {
		t.Errorf("expected filter_field 'email', got %q", note.Metadata["filter_field"])
	}
}

func TestDiscover_NoResources(t *testing.T) {
	ctx := context.Background()
	mem := newMockMemory()
	thought, _ := New(ctx, mem, "test")

	catalog := newMockCatalog()
	// No resources
	ctx = WithCatalog(ctx, catalog)

	discover := NewDiscover("sources")
	result, err := discover.Process(ctx, thought)
	if err != nil {
		t.Fatalf("discover failed: %v", err)
	}

	content, _ := result.GetContent("sources")
	if content != NoResourcesFound {
		t.Errorf("expected %q, got %q", NoResourcesFound, content)
	}
}

func TestDiscover_NoCatalog(t *testing.T) {
	SetCatalog(nil)

	ctx := context.Background()
	mem := newMockMemory()
	thought, _ := New(ctx, mem, "test")

	discover := NewDiscover("sources")
	_, err := discover.Process(ctx, thought)
	if err == nil {
		t.Error("expected error when no catalog is configured")
	}

	if !errors.Is(err, ErrNoCatalog) {
		t.Errorf("expected ErrNoCatalog, got: %v", err)
	}
}

func TestDiscover_WithCatalog(t *testing.T) {
	ctx := context.Background()
	mem := newMockMemory()
	thought, _ := New(ctx, mem, "test")

	// Set global (should be ignored)
	globalCatalog := newMockCatalog()
	globalCatalog.resources = []CatalogResource{
		{URI: "db://global", Variant: "db"},
	}
	SetCatalog(globalCatalog)
	defer SetCatalog(nil)

	// Step-level catalog
	stepCatalog := newMockCatalog()
	stepCatalog.resources = []CatalogResource{
		{URI: "db://step", Variant: "db"},
	}

	discover := NewDiscover("sources").WithCatalog(stepCatalog)
	result, err := discover.Process(ctx, thought)
	if err != nil {
		t.Fatalf("discover failed: %v", err)
	}

	content, _ := result.GetContent("sources")
	if !containsString(content, "db://step") {
		t.Error("expected db://step from step catalog")
	}
	if containsString(content, "db://global") {
		t.Error("should not have db://global (step catalog should take precedence)")
	}
}

func TestDiscover_Identity(t *testing.T) {
	discover := NewDiscover("my_discover")
	if discover.Identity().Name() != "my_discover" {
		t.Errorf("expected name 'my_discover', got %q", discover.Identity().Name())
	}
}

func TestDiscover_Schema(t *testing.T) {
	discover := NewDiscover("test")
	schema := discover.Schema()
	if schema.Type != "discover" {
		t.Errorf("expected type 'discover', got %q", schema.Type)
	}
}

func TestDiscover_Close(t *testing.T) {
	discover := NewDiscover("test")
	if err := discover.Close(); err != nil {
		t.Errorf("unexpected error from Close: %v", err)
	}
}
