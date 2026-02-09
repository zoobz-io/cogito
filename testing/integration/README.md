# Integration Tests

End-to-end tests that require external dependencies (database, LLM providers).

## Running Integration Tests

```bash
# Run integration tests
make test-integration

# Or directly with go test
go test -v -race -tags=integration ./testing/integration/...
```

## Test Structure

Integration tests use the `integration` build tag:

```go
//go:build integration

package integration_test
```

This ensures they don't run during normal `go test` execution.

## Writing Integration Tests

```go
//go:build integration

package integration_test

import (
    "context"
    "testing"

    "github.com/zoobzio/cogito"
)

func TestMemoryImplementation(t *testing.T) {
    // Set up your Memory implementation
    memory := setupTestMemory(t)

    ctx := context.Background()
    thought, err := cogito.New(ctx, memory, "test intent")
    if err != nil {
        t.Fatalf("failed to create thought: %v", err)
    }

    // Test cases...
    _ = memory.DeleteThought(ctx, thought.ID)
}
```
