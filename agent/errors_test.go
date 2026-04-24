package agent

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"pgregory.net/rapid"
)

// testSentinelError is a custom error type used to verify errors.As works through chains.
type testSentinelError struct{ msg string }

func (e *testSentinelError) Error() string { return e.msg }

// newAnthropicError constructs an *anthropic.Error with the given status code
// and a JSON body containing the message, suitable for testing wrapAPIError.
func newAnthropicError(statusCode int, message string) *anthropic.Error {
	// Build a minimal HTTP request and response so that anthropic.Error methods work.
	req := httptest.NewRequest(http.MethodPost, "https://api.example.com/v1/messages", nil)
	resp := &http.Response{
		StatusCode: statusCode,
		Status:     fmt.Sprintf("%d %s", statusCode, http.StatusText(statusCode)),
	}

	apiErr := &anthropic.Error{
		StatusCode: statusCode,
		Request:    req,
		Response:   resp,
	}
	// UnmarshalJSON populates the internal raw JSON field returned by RawJSON().
	// The body must be valid JSON for the SDK decoder to store it.
	jsonBody := fmt.Sprintf(`{"type":"error","error":{"type":"api_error","message":"%s"}}`, message)
	_ = apiErr.UnmarshalJSON([]byte(jsonBody))
	return apiErr
}

// Feature: ardp-coding-agent, Property 16: HTTP errors contain status and body
// Validates: Requirements 10.1
func TestHTTPErrorContent(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		statusCode := rapid.IntRange(400, 599).Draw(t, "statusCode")
		// Use alphanumeric messages safe for JSON string embedding
		message := rapid.StringMatching(`[a-zA-Z0-9 ]{1,60}`).Draw(t, "message")

		apiErr := newAnthropicError(statusCode, message)
		wrapped := wrapAPIError(apiErr)

		errStr := wrapped.Error()
		statusStr := fmt.Sprintf("%d", statusCode)
		if !strings.Contains(errStr, statusStr) {
			t.Fatalf("wrapped error %q does not contain status code %q", errStr, statusStr)
		}
		if !strings.Contains(errStr, message) {
			t.Fatalf("wrapped error %q does not contain message %q", errStr, message)
		}
	})
}

// TestHTTPErrorNonAPIError verifies that wrapAPIError wraps non-Anthropic errors
// with "api call failed" context.
func TestHTTPErrorNonAPIError(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		msg := rapid.StringMatching(`[a-zA-Z0-9 ]{1,50}`).Draw(t, "errMsg")
		err := errors.New(msg)
		wrapped := wrapAPIError(err)

		errStr := wrapped.Error()
		if !strings.Contains(errStr, "api call failed") {
			t.Fatalf("wrapped error %q does not contain 'api call failed'", errStr)
		}
		if !strings.Contains(errStr, msg) {
			t.Fatalf("wrapped error %q does not contain original message %q", errStr, msg)
		}
		// Verify the original error is preserved in the chain
		if !errors.Is(wrapped, err) {
			t.Fatalf("errors.Is failed: wrapped error does not contain original")
		}
	})
}

// Feature: ardp-coding-agent, Property 17: Errors wrapped with operation context
// Validates: Requirements 10.3
func TestErrorOperationContext(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		operation := rapid.StringMatching(`[a-zA-Z ]{1,30}`).Draw(t, "operation")
		errMsg := rapid.StringMatching(`[a-zA-Z0-9 ]{1,50}`).Draw(t, "errMsg")

		original := errors.New(errMsg)
		wrapped := wrapOperationError(operation, original)

		errStr := wrapped.Error()
		if !strings.Contains(errStr, operation) {
			t.Fatalf("wrapped error %q does not contain operation %q", errStr, operation)
		}
		if !strings.Contains(errStr, errMsg) {
			t.Fatalf("wrapped error %q does not contain original message %q", errStr, errMsg)
		}
	})
}

// --- Unit tests for error handling (Task 11.4) ---

// TestContextCancellationError verifies that wrapping a context.Canceled error
// preserves detectability via errors.Is.
func TestContextCancellationError(t *testing.T) {
	wrapped := wrapOperationError("api call", context.Canceled)
	if !errors.Is(wrapped, context.Canceled) {
		t.Fatal("errors.Is(wrapped, context.Canceled) should be true")
	}
	if !strings.Contains(wrapped.Error(), "api call") {
		t.Fatal("wrapped error should contain operation context 'api call'")
	}
}

// TestErrorChainPreserved verifies that errors.Is and errors.As work through
// the wrapOperationError chain.
func TestErrorChainPreserved(t *testing.T) {
	// Create a sentinel error type
	sentinel := &testSentinelError{msg: "root cause"}

	// Wrap it as a regular error first
	base := fmt.Errorf("base: %w", sentinel)
	wrapped := wrapOperationError("tool execution", base)

	// errors.As should find the sentinel through the chain
	var target *testSentinelError
	if !errors.As(wrapped, &target) {
		t.Fatal("errors.As should find sentinelError through the chain")
	}
	if target.msg != "root cause" {
		t.Fatalf("expected 'root cause', got %q", target.msg)
	}

	// errors.Is should find base through the chain
	if !errors.Is(wrapped, sentinel) {
		t.Fatal("errors.Is should find sentinel through the chain")
	}
}

// TestWrapToolErrorPreservesChain verifies that wrapToolError preserves the
// original error in the chain for errors.Is / errors.As inspection.
func TestWrapToolErrorPreservesChain(t *testing.T) {
	original := errors.New("disk full")
	wrapped := wrapToolError("write_file", 3, original)

	// The original error should be findable via errors.Is
	if !errors.Is(wrapped, original) {
		t.Fatal("errors.Is should find original error through wrapToolError chain")
	}

	// The wrapped error should contain tool name and iteration
	errStr := wrapped.Error()
	if !strings.Contains(errStr, "write_file") {
		t.Fatalf("wrapped error %q should contain tool name 'write_file'", errStr)
	}
	if !strings.Contains(errStr, "3") {
		t.Fatalf("wrapped error %q should contain iteration '3'", errStr)
	}
}
