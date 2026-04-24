package elicitation

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ConversationMarshaler handles conversation markdown serialization.
type ConversationMarshaler interface {
	Marshal(session *Session) ([]byte, error)
	Unmarshal(data []byte) (*Session, error)
}

// markdownFrontMatter represents the YAML front-matter in a conversation markdown file.
type markdownFrontMatter struct {
	SessionID    string `yaml:"session_id"`
	Name         string `yaml:"name"`
	Persona      string `yaml:"persona"`
	CreatedAt    string `yaml:"created_at"`
	MessageCount int    `yaml:"message_count"`
}

// markdownCodec implements ConversationMarshaler.
type markdownCodec struct{}

// NewMarkdownCodec creates a new ConversationMarshaler.
func NewMarkdownCodec() ConversationMarshaler {
	return &markdownCodec{}
}

// Marshal serializes a Session to conversation markdown with YAML front-matter.
func (c *markdownCodec) Marshal(session *Session) ([]byte, error) {
	var buf bytes.Buffer

	// Write YAML front-matter
	fm := markdownFrontMatter{
		SessionID:    session.ID,
		Name:         session.Name,
		Persona:      string(session.Persona),
		CreatedAt:    session.CreatedAt.UTC().Format(time.RFC3339),
		MessageCount: len(session.Messages),
	}

	yamlBytes, err := yaml.Marshal(&fm)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal front-matter: %w", err)
	}

	buf.WriteString("---\n")
	buf.Write(yamlBytes)
	buf.WriteString("---\n")

	// Write messages
	for _, msg := range session.Messages {
		header := msg.Role
		if msg.Role == "assistant" {
			// Use PersonaName for the header; fall back to PersonaDisplayNames
			header = msg.PersonaName
			if header == "" {
				header = PersonaDisplayNames[session.Persona]
				if header == "" {
					header = "assistant"
				}
			}
		}
		buf.WriteString(fmt.Sprintf("\n## %s — %s\n\n", header, msg.SentAt.UTC().Format(time.RFC3339)))
		buf.WriteString(msg.Content)
		buf.WriteString("\n")
	}

	return buf.Bytes(), nil
}

// Unmarshal parses conversation markdown back into a Session.
func (c *markdownCodec) Unmarshal(data []byte) (*Session, error) {
	text := string(data)

	// Extract YAML front-matter between --- delimiters
	if !strings.HasPrefix(text, "---\n") {
		return nil, &ParseError{
			Section: "front-matter",
			Cause:   fmt.Errorf("%w: missing opening front-matter delimiter", ErrMarkdownParse),
		}
	}

	// Find the closing ---
	rest := text[4:] // skip opening "---\n"
	closingIdx := strings.Index(rest, "\n---\n")
	if closingIdx == -1 {
		// Also check for --- at end of string (no trailing newline after ---)
		if strings.HasSuffix(rest, "\n---") {
			closingIdx = len(rest) - 4
		} else {
			return nil, &ParseError{
				Section: "front-matter",
				Cause:   fmt.Errorf("%w: missing closing front-matter delimiter", ErrMarkdownParse),
			}
		}
	}

	yamlContent := rest[:closingIdx]
	body := rest[closingIdx+4:] // skip "\n---\n" or "\n---"
	if strings.HasPrefix(body, "\n") {
		// normalize: body starts after "---\n"
	}

	// Parse YAML front-matter
	var fm markdownFrontMatter
	if err := yaml.Unmarshal([]byte(yamlContent), &fm); err != nil {
		return nil, &ParseError{
			Section: "front-matter",
			Cause:   fmt.Errorf("%w: %v", ErrMarkdownParse, err),
		}
	}

	// Validate required fields
	if fm.SessionID == "" {
		return nil, &ParseError{
			Section: "front-matter",
			Cause:   fmt.Errorf("%w: missing session_id", ErrMarkdownParse),
		}
	}
	if fm.Persona == "" {
		return nil, &ParseError{
			Section: "front-matter",
			Cause:   fmt.Errorf("%w: missing persona", ErrMarkdownParse),
		}
	}

	// Default name if missing
	if fm.Name == "" {
		fm.Name = "Untitled Session"
	}

	// Parse created_at
	createdAt, err := time.Parse(time.RFC3339, fm.CreatedAt)
	if err != nil {
		return nil, &ParseError{
			Section: "front-matter",
			Cause:   fmt.Errorf("%w: invalid created_at: %v", ErrMarkdownParse, err),
		}
	}

	// Parse messages by splitting on "## " headers
	messages, err := parseMessages(body)
	if err != nil {
		return nil, err
	}

	return &Session{
		ID:        fm.SessionID,
		Name:      fm.Name,
		Persona:   PersonaType(fm.Persona),
		CreatedAt: createdAt,
		Messages:  messages,
	}, nil
}

// parseMessages splits the body on "## " headers and extracts role, timestamp, and content.
func parseMessages(body string) ([]ChatMessage, error) {
	// Trim only leading/trailing newlines from body
	body = strings.Trim(body, "\n\r")
	if body == "" {
		return []ChatMessage{}, nil
	}

	// Split on "\n## " to find message sections
	sections := strings.Split(body, "\n## ")

	var messages []ChatMessage
	for i, section := range sections {
		// Skip empty leading section
		trimmed := strings.Trim(section, "\n\r")
		if trimmed == "" {
			continue
		}

		// The first section might start with "## " (no leading newline)
		if i == 0 && strings.HasPrefix(section, "## ") {
			section = section[3:]
		}

		// Parse header: "role — timestamp"
		lines := strings.SplitN(section, "\n", 2)
		header := strings.TrimSpace(lines[0])

		// Split on " — " (em dash with spaces)
		parts := strings.SplitN(header, " — ", 2)
		if len(parts) != 2 {
			return nil, &ParseError{
				Section: "message",
				Cause:   fmt.Errorf("%w: invalid message header format: %q", ErrMarkdownParse, header),
			}
		}

		role := strings.TrimSpace(parts[0])
		timestampStr := strings.TrimSpace(parts[1])

		sentAt, err := time.Parse(time.RFC3339, timestampStr)
		if err != nil {
			return nil, &ParseError{
				Section: "message",
				Cause:   fmt.Errorf("%w: invalid timestamp %q: %v", ErrMarkdownParse, timestampStr, err),
			}
		}

		content := ""
		if len(lines) > 1 {
			// Trim only leading/trailing newlines; preserve spaces within content
			content = strings.Trim(lines[1], "\n\r")
		}

		// Determine role and persona name from header text
		var msgRole, personaName string
		switch role {
		case "user":
			msgRole = "user"
		case "system":
			msgRole = "system"
		default:
			// Any other header text is an assistant message; the header text is the persona name
			msgRole = "assistant"
			personaName = role
		}

		messages = append(messages, ChatMessage{
			Role:        msgRole,
			PersonaName: personaName,
			Content:     content,
			SentAt:      sentAt,
		})
	}

	return messages, nil
}
