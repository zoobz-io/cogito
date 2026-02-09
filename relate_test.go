package cogito

import (
	"context"
	"errors"
	"testing"
)

func TestRelate_FindsRelatedResources(t *testing.T) {
	ctx := context.Background()
	mem := newMockMemory()
	thought, _ := New(ctx, mem, "test")

	catalog := newMockCatalog()
	catalog.resources = []CatalogResource{
		{URI: "db://users", Variant: "db", Description: "Primary user store"},
		{URI: "kv://user_cache", Variant: "kv", Description: "User cache"},
		{URI: "idx://user_embeddings", Variant: "idx", Description: "User embeddings"},
	}
	ctx = WithCatalog(ctx, catalog)

	relate := NewRelate("user_sources", "db://users")
	result, err := relate.Process(ctx, thought)
	if err != nil {
		t.Fatalf("relate failed: %v", err)
	}

	content, err := result.GetContent("user_sources")
	if err != nil {
		t.Fatalf("expected content: %v", err)
	}

	// Mock returns all resources except first as "related"
	if !containsString(content, "kv://user_cache") {
		t.Error("expected kv://user_cache in related resources")
	}
	if !containsString(content, "idx://user_embeddings") {
		t.Error("expected idx://user_embeddings in related resources")
	}

	// Check metadata
	note, _ := result.GetNote("user_sources")
	if note.Metadata["source_uri"] != "db://users" {
		t.Errorf("expected source_uri 'db://users', got %q", note.Metadata["source_uri"])
	}
	if note.Metadata["related_count"] != "2" {
		t.Errorf("expected related_count '2', got %q", note.Metadata["related_count"])
	}
}

func TestRelate_NoRelatedResources(t *testing.T) {
	ctx := context.Background()
	mem := newMockMemory()
	thought, _ := New(ctx, mem, "test")

	catalog := newMockCatalog()
	// Only one resource, so no related ones
	catalog.resources = []CatalogResource{
		{URI: "db://users", Variant: "db"},
	}
	ctx = WithCatalog(ctx, catalog)

	relate := NewRelate("user_sources", "db://users")
	result, err := relate.Process(ctx, thought)
	if err != nil {
		t.Fatalf("relate failed: %v", err)
	}

	content, _ := result.GetContent("user_sources")
	if content != NoResourcesFound {
		t.Errorf("expected %q, got %q", NoResourcesFound, content)
	}

	note, _ := result.GetNote("user_sources")
	if note.Metadata["related_count"] != "0" {
		t.Errorf("expected related_count '0', got %q", note.Metadata["related_count"])
	}
}

func TestRelate_EmptyCatalog(t *testing.T) {
	ctx := context.Background()
	mem := newMockMemory()
	thought, _ := New(ctx, mem, "test")

	catalog := newMockCatalog()
	// No resources at all
	ctx = WithCatalog(ctx, catalog)

	relate := NewRelate("user_sources", "db://users")
	result, err := relate.Process(ctx, thought)
	if err != nil {
		t.Fatalf("relate failed: %v", err)
	}

	content, _ := result.GetContent("user_sources")
	if content != NoResourcesFound {
		t.Errorf("expected %q, got %q", NoResourcesFound, content)
	}
}

func TestRelate_NoCatalog(t *testing.T) {
	SetCatalog(nil)

	ctx := context.Background()
	mem := newMockMemory()
	thought, _ := New(ctx, mem, "test")

	relate := NewRelate("user_sources", "db://users")
	_, err := relate.Process(ctx, thought)
	if err == nil {
		t.Error("expected error when no catalog is configured")
	}

	if !errors.Is(err, ErrNoCatalog) {
		t.Errorf("expected ErrNoCatalog, got: %v", err)
	}
}

func TestRelate_WithCatalog(t *testing.T) {
	ctx := context.Background()
	mem := newMockMemory()
	thought, _ := New(ctx, mem, "test")

	// Set global (should be ignored)
	globalCatalog := newMockCatalog()
	globalCatalog.resources = []CatalogResource{
		{URI: "db://global", Variant: "db"},
		{URI: "kv://global_cache", Variant: "kv"},
	}
	SetCatalog(globalCatalog)
	defer SetCatalog(nil)

	// Step-level catalog
	stepCatalog := newMockCatalog()
	stepCatalog.resources = []CatalogResource{
		{URI: "db://step", Variant: "db"},
		{URI: "kv://step_cache", Variant: "kv"},
	}

	relate := NewRelate("sources", "db://step").WithCatalog(stepCatalog)
	result, err := relate.Process(ctx, thought)
	if err != nil {
		t.Fatalf("relate failed: %v", err)
	}

	content, _ := result.GetContent("sources")
	if !containsString(content, "kv://step_cache") {
		t.Error("expected kv://step_cache from step catalog")
	}
	if containsString(content, "kv://global_cache") {
		t.Error("should not have kv://global_cache")
	}
}

func TestRelate_Identity(t *testing.T) {
	relate := NewRelate("my_relate", "db://users")
	if relate.Identity().Name() != "my_relate" {
		t.Errorf("expected name 'my_relate', got %q", relate.Identity().Name())
	}
}

func TestRelate_Schema(t *testing.T) {
	relate := NewRelate("test", "db://users")
	schema := relate.Schema()
	if schema.Type != "relate" {
		t.Errorf("expected type 'relate', got %q", schema.Type)
	}
}

func TestRelate_Close(t *testing.T) {
	relate := NewRelate("test", "db://users")
	if err := relate.Close(); err != nil {
		t.Errorf("unexpected error from Close: %v", err)
	}
}
