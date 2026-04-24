package agent

import (
	"errors"
	"fmt"

	anthropic "github.com/anthropics/anthropic-sdk-go"
)

// wrapAPIError wraps an API error with HTTP status code and response body
// if the underlying error is an Anthropic SDK API error. Otherwise, it wraps
// the error with a generic "api call failed" context.
func wrapAPIError(err error) error {
	var apiErr *anthropic.Error
	if errors.As(err, &apiErr) {
		return fmt.Errorf("api call failed [status=%d]: %s: %w", apiErr.StatusCode, apiErr.RawJSON(), err)
	}
	return fmt.Errorf("api call failed: %w", err)
}

// wrapOperationError wraps any error with the operation context string,
// e.g. "prompt assembly", "api call", "tool execution".
func wrapOperationError(operation string, err error) error {
	return fmt.Errorf("%s failed: %w", operation, err)
}

// wrapToolError wraps a tool execution error with the tool name and
// iteration number for diagnostics.
func wrapToolError(toolName string, iteration int, err error) error {
	return fmt.Errorf("tool execution failed [tool=%s, iteration=%d]: %w", toolName, iteration, err)
}
