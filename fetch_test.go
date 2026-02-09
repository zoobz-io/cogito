package cogito

import (
	"context"
	"errors"
	"testing"

	"github.com/zoobzio/atom"
)

func TestFetch_StaticURI(t *testing.T) {
	ctx := context.Background()
	mem := newMockMemory()
	thought, _ := New(ctx, mem, "test")

	// Set up mock catalog with data
	catalog := newMockCatalog()
	catalog.data["db://users/alice"] = &atom.Atom{
		Strings: map[string]string{"name": "Alice", "email": "alice@example.com"},
		Ints:    map[string]int64{"age": 30},
	}
	ctx = WithCatalog(ctx, catalog)

	fetch := NewFetch("user", "db://users/alice")
	result, err := fetch.Process(ctx, thought)
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}

	// Check note was created
	content, err := result.GetContent("user")
	if err != nil {
		t.Fatalf("expected content at 'user': %v", err)
	}

	// Check content contains expected data
	if !containsString(content, "Alice") {
		t.Errorf("expected 'Alice' in content, got: %s", content)
	}
	if !containsString(content, "alice@example.com") {
		t.Errorf("expected email in content, got: %s", content)
	}

	// Check metadata
	note, ok := result.GetNote("user")
	if !ok {
		t.Fatal("expected note at 'user'")
	}
	if note.Metadata["source_uri"] != "db://users/alice" {
		t.Errorf("expected source_uri metadata, got: %v", note.Metadata)
	}
}

func TestFetch_DynamicURI(t *testing.T) {
	ctx := context.Background()
	mem := newMockMemory()
	thought, _ := New(ctx, mem, "test")

	// Set user_id in thought
	thought.SetContent(ctx, "user_id", "bob", "input")

	// Set up mock catalog
	catalog := newMockCatalog()
	catalog.data["db://users/bob"] = &atom.Atom{
		Strings: map[string]string{"name": "Bob"},
	}
	ctx = WithCatalog(ctx, catalog)

	fetch := NewFetchDynamic("user", "db://users/", "user_id")
	result, err := fetch.Process(ctx, thought)
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}

	content, err := result.GetContent("user")
	if err != nil {
		t.Fatalf("expected content: %v", err)
	}

	if !containsString(content, "Bob") {
		t.Errorf("expected 'Bob' in content, got: %s", content)
	}

	// Check metadata has correct URI
	note, _ := result.GetNote("user")
	if note.Metadata["source_uri"] != "db://users/bob" {
		t.Errorf("expected source_uri 'db://users/bob', got: %s", note.Metadata["source_uri"])
	}
}

func TestFetch_DynamicURI_MissingKey(t *testing.T) {
	ctx := context.Background()
	mem := newMockMemory()
	thought, _ := New(ctx, mem, "test")

	// Don't set user_id
	catalog := newMockCatalog()
	ctx = WithCatalog(ctx, catalog)

	fetch := NewFetchDynamic("user", "db://users/", "user_id")
	_, err := fetch.Process(ctx, thought)
	if err == nil {
		t.Error("expected error when URI key is missing")
	}
}

func TestFetch_NotFound(t *testing.T) {
	ctx := context.Background()
	mem := newMockMemory()
	thought, _ := New(ctx, mem, "test")

	catalog := newMockCatalog()
	// Don't add any data
	ctx = WithCatalog(ctx, catalog)

	fetch := NewFetch("user", "db://users/nobody")
	_, err := fetch.Process(ctx, thought)
	if err == nil {
		t.Error("expected error when data not found")
	}
}

func TestFetch_NoCatalog(t *testing.T) {
	// Clear global catalog
	SetCatalog(nil)

	ctx := context.Background()
	mem := newMockMemory()
	thought, _ := New(ctx, mem, "test")

	fetch := NewFetch("user", "db://users/alice")
	_, err := fetch.Process(ctx, thought)
	if err == nil {
		t.Error("expected error when no catalog is configured")
	}

	if !errors.Is(err, ErrNoCatalog) {
		t.Errorf("expected ErrNoCatalog, got: %v", err)
	}
}

func TestFetch_WithCatalog(t *testing.T) {
	ctx := context.Background()
	mem := newMockMemory()
	thought, _ := New(ctx, mem, "test")

	// Set global catalog (should be ignored)
	globalCatalog := newMockCatalog()
	SetCatalog(globalCatalog)
	defer SetCatalog(nil)

	// Create step-level catalog with data
	stepCatalog := newMockCatalog()
	stepCatalog.data["db://users/alice"] = &atom.Atom{
		Strings: map[string]string{"name": "Step Alice"},
	}

	fetch := NewFetch("user", "db://users/alice").WithCatalog(stepCatalog)
	result, err := fetch.Process(ctx, thought)
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}

	content, _ := result.GetContent("user")
	if !containsString(content, "Step Alice") {
		t.Errorf("expected 'Step Alice' from step catalog, got: %s", content)
	}
}

func TestFetch_CatalogError(t *testing.T) {
	ctx := context.Background()
	mem := newMockMemory()
	thought, _ := New(ctx, mem, "test")

	catalog := newMockCatalog()
	catalog.getErr = errors.New("connection failed")
	ctx = WithCatalog(ctx, catalog)

	fetch := NewFetch("user", "db://users/alice")
	_, err := fetch.Process(ctx, thought)
	if err == nil {
		t.Error("expected error when catalog returns error")
	}
}

func TestFetch_Identity(t *testing.T) {
	fetch := NewFetch("my_fetch", "db://users/alice")
	if fetch.Identity().Name() != "my_fetch" {
		t.Errorf("expected name 'my_fetch', got %q", fetch.Identity().Name())
	}
}

func TestFetch_Schema(t *testing.T) {
	fetch := NewFetch("test", "db://users/alice")
	schema := fetch.Schema()
	if schema.Type != "fetch" {
		t.Errorf("expected type 'fetch', got %q", schema.Type)
	}
}

func TestFetch_Close(t *testing.T) {
	fetch := NewFetch("test", "db://users/alice")
	if err := fetch.Close(); err != nil {
		t.Errorf("unexpected error from Close: %v", err)
	}
}
