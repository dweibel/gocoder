package elicitation

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// --- Task 9.1: Tests for error types and sentinel errors ---
// Validates: Requirements 3.4, 7.3, 9.5, 13.5

// TestSentinelErrors verifies that all sentinel errors are distinct, non-nil,
// and contain expected substrings in their messages.
func TestSentinelErrors(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		contains string
	}{
		{"ErrSessionNotFound", ErrSessionNotFound, "session not found"},
		{"ErrInvalidPersona", ErrInvalidPersona, "invalid persona"},
		{"ErrLLMTimeout", ErrLLMTimeout, "timed out"},
		{"ErrSynthesisParse", ErrSynthesisParse, "synthesis"},
		{"ErrMarkdownParse", ErrMarkdownParse, "markdown"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.err == nil {
				t.Fatal("sentinel error should not be nil")
			}
			if !strings.Contains(strings.ToLower(tc.err.Error()), tc.contains) {
				t.Fatalf("error %q should contain %q", tc.err.Error(), tc.contains)
			}
		})
	}
}

// TestSentinelErrorsAreDistinct verifies that no two sentinel errors are equal.
func TestSentinelErrorsAreDistinct(t *testing.T) {
	sentinels := []error{
		ErrSessionNotFound,
		ErrInvalidPersona,
		ErrLLMTimeout,
		ErrSynthesisParse,
		ErrMarkdownParse,
	}

	for i := 0; i < len(sentinels); i++ {
		for j := i + 1; j < len(sentinels); j++ {
			if errors.Is(sentinels[i], sentinels[j]) {
				t.Fatalf("sentinel errors at index %d and %d should not be equal", i, j)
			}
		}
	}
}

// TestSentinelErrorsMatchWithErrorsIs verifies that errors.Is works correctly
// for each sentinel error (identity check).
func TestSentinelErrorsMatchWithErrorsIs(t *testing.T) {
	sentinels := []error{
		ErrSessionNotFound,
		ErrInvalidPersona,
		ErrLLMTimeout,
		ErrSynthesisParse,
		ErrMarkdownParse,
	}

	for _, s := range sentinels {
		wrapped := fmt.Errorf("context: %w", s)
		if !errors.Is(wrapped, s) {
			t.Fatalf("errors.Is should find %v through wrapping", s)
		}
	}
}

// TestParseErrorFormat verifies that ParseError.Error() produces the expected
// format: `parse error in section "SECTION": cause message`.
func TestParseErrorFormat(t *testing.T) {
	cause := errors.New("unexpected delimiter")
	pe := &ParseError{Section: "BRD", Cause: cause}

	got := pe.Error()
	expected := `parse error in section "BRD": unexpected delimiter`
	if got != expected {
		t.Fatalf("ParseError.Error() = %q, want %q", got, expected)
	}
}

// TestParseErrorFormatVariousSections verifies Error() format across different sections.
func TestParseErrorFormatVariousSections(t *testing.T) {
	sections := []string{"BRD", "SRS", "NFR", "GHERKIN", "front-matter"}

	for _, section := range sections {
		t.Run(section, func(t *testing.T) {
			cause := fmt.Errorf("bad content in %s", section)
			pe := &ParseError{Section: section, Cause: cause}

			got := pe.Error()
			if !strings.Contains(got, fmt.Sprintf("%q", section)) {
				t.Fatalf("Error() = %q should contain quoted section %q", got, section)
			}
			if !strings.Contains(got, cause.Error()) {
				t.Fatalf("Error() = %q should contain cause %q", got, cause.Error())
			}
		})
	}
}

// TestParseErrorUnwrap verifies that Unwrap returns the underlying cause and
// that errors.Is can find the cause through the ParseError wrapper.
func TestParseErrorUnwrap(t *testing.T) {
	cause := errors.New("missing end delimiter")
	pe := &ParseError{Section: "SRS", Cause: cause}

	unwrapped := pe.Unwrap()
	if unwrapped != cause {
		t.Fatalf("Unwrap() returned %v, want %v", unwrapped, cause)
	}

	if !errors.Is(pe, cause) {
		t.Fatal("errors.Is(ParseError, cause) should be true")
	}
}

// TestParseErrorUnwrapNilCause verifies that Unwrap returns nil when Cause is nil.
func TestParseErrorUnwrapNilCause(t *testing.T) {
	pe := &ParseError{Section: "GHERKIN", Cause: nil}

	if pe.Unwrap() != nil {
		t.Fatal("Unwrap() should return nil when Cause is nil")
	}
}

// TestParseErrorAsTarget verifies that errors.As can extract a *ParseError
// from a wrapped error chain.
func TestParseErrorAsTarget(t *testing.T) {
	cause := errors.New("corrupt data")
	pe := &ParseError{Section: "NFR", Cause: cause}
	wrapped := fmt.Errorf("synthesis failed: %w", pe)

	var target *ParseError
	if !errors.As(wrapped, &target) {
		t.Fatal("errors.As should find *ParseError through wrapping")
	}
	if target.Section != "NFR" {
		t.Fatalf("target.Section = %q, want %q", target.Section, "NFR")
	}
	if target.Cause != cause {
		t.Fatal("target.Cause should be the original cause")
	}
}

// TestParseErrorImplementsErrorInterface verifies ParseError satisfies the error interface.
func TestParseErrorImplementsErrorInterface(t *testing.T) {
	var _ error = &ParseError{}
}
