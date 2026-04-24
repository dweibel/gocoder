package elicitation

import (
	"fmt"
	"strings"
)

// ArtifactType constants identify the kind of synthesized artifact.
const (
	ArtifactBRD     = "BRD"
	ArtifactSRS     = "SRS"
	ArtifactNFR     = "NFR"
	ArtifactGherkin = "GHERKIN"
)

// requiredSections defines the sections expected in synthesis output, in order.
var requiredSections = []string{ArtifactBRD, ArtifactSRS, ArtifactNFR, ArtifactGherkin}

// ArtifactParser parses raw synthesis output into typed artifacts.
type ArtifactParser interface {
	Parse(raw string) ([]Artifact, error)
}

// ArtifactSerializer serializes artifacts back to the delimited text format.
type ArtifactSerializer interface {
	Serialize(artifacts []Artifact) (string, error)
}

// artifactCodec implements both ArtifactParser and ArtifactSerializer.
type artifactCodec struct{}

// NewArtifactCodec creates a new artifact parser/serializer.
func NewArtifactCodec() *artifactCodec {
	return &artifactCodec{}
}

// Parse splits raw synthesis output on ==={TYPE}=== / ===END_{TYPE}=== delimiters
// and returns typed Artifact records. Gherkin stories are further split on
// "### Story:" headers.
func (c *artifactCodec) Parse(raw string) ([]Artifact, error) {
	var artifacts []Artifact

	for _, section := range requiredSections {
		startDelim := fmt.Sprintf("===%s===", section)
		endDelim := fmt.Sprintf("===END_%s===", section)

		startIdx := strings.Index(raw, startDelim)
		if startIdx == -1 {
			return nil, &ParseError{
				Section: section,
				Cause:   fmt.Errorf("%w: missing start delimiter %q", ErrSynthesisParse, startDelim),
			}
		}

		endIdx := strings.Index(raw, endDelim)
		if endIdx == -1 {
			return nil, &ParseError{
				Section: section,
				Cause:   fmt.Errorf("%w: missing end delimiter %q", ErrSynthesisParse, endDelim),
			}
		}

		if endIdx <= startIdx {
			return nil, &ParseError{
				Section: section,
				Cause:   fmt.Errorf("%w: end delimiter before start delimiter", ErrSynthesisParse),
			}
		}

		content := raw[startIdx+len(startDelim) : endIdx]
		content = strings.TrimSpace(content)

		if section == ArtifactGherkin {
			stories, err := parseGherkinStories(content)
			if err != nil {
				return nil, &ParseError{
					Section: section,
					Cause:   err,
				}
			}
			artifacts = append(artifacts, stories...)
		} else {
			artifacts = append(artifacts, Artifact{
				Type:    section,
				Title:   section,
				Content: content,
			})
		}
	}

	return artifacts, nil
}

// parseGherkinStories splits Gherkin content on "### Story:" headers.
func parseGherkinStories(content string) ([]Artifact, error) {
	parts := strings.Split(content, "### Story:")
	var stories []Artifact

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// First line is the title, rest is the story content
		lines := strings.SplitN(part, "\n", 2)
		title := strings.TrimSpace(lines[0])
		storyContent := ""
		if len(lines) > 1 {
			storyContent = strings.TrimSpace(lines[1])
		}

		stories = append(stories, Artifact{
			Type:    ArtifactGherkin,
			Title:   title,
			Content: storyContent,
		})
	}

	if len(stories) == 0 {
		return nil, fmt.Errorf("%w: no stories found in GHERKIN section", ErrSynthesisParse)
	}

	return stories, nil
}

// Serialize produces the delimited text format from a slice of artifacts.
// It groups artifacts by type and produces ==={TYPE}=== / ===END_{TYPE}=== blocks.
func (c *artifactCodec) Serialize(artifacts []Artifact) (string, error) {
	// Group artifacts by type, preserving order
	grouped := make(map[string][]Artifact)
	for _, a := range artifacts {
		grouped[a.Type] = append(grouped[a.Type], a)
	}

	var sb strings.Builder

	for _, section := range requiredSections {
		items, ok := grouped[section]
		if !ok {
			continue
		}

		sb.WriteString(fmt.Sprintf("===%s===\n", section))

		if section == ArtifactGherkin {
			for i, a := range items {
				if i > 0 {
					sb.WriteString("\n")
				}
				sb.WriteString(fmt.Sprintf("### Story: %s\n", a.Title))
				sb.WriteString(a.Content)
				sb.WriteString("\n")
			}
		} else {
			// For non-Gherkin sections, there should be exactly one artifact
			sb.WriteString(items[0].Content)
			sb.WriteString("\n")
		}

		sb.WriteString(fmt.Sprintf("===END_%s===\n\n", section))
	}

	return strings.TrimRight(sb.String(), "\n"), nil
}
