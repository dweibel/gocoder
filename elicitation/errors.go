package elicitation

import (
	"errors"
	"fmt"
)

// ErrSessionNotFound is returned when a session ID does not match any active session.
var ErrSessionNotFound = errors.New("session not found")

// ErrInvalidPersona is returned when an unknown persona type is requested.
var ErrInvalidPersona = errors.New("invalid persona type")

// ErrLLMTimeout is returned when the LLM request exceeds the configured timeout.
var ErrLLMTimeout = errors.New("LLM request timed out")

// ErrSynthesisParse is returned when synthesis output cannot be parsed.
var ErrSynthesisParse = errors.New("synthesis output parse failed")

// ErrMarkdownParse is returned when conversation markdown cannot be parsed.
var ErrMarkdownParse = errors.New("conversation markdown parse failed")

// ParseError wraps a parse failure with the section that failed.
type ParseError struct {
	Section string // e.g., "BRD", "SRS", "GHERKIN", "front-matter"
	Cause   error
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("parse error in section %q: %v", e.Section, e.Cause)
}

func (e *ParseError) Unwrap() error {
	return e.Cause
}
