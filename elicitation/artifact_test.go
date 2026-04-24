package elicitation

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// ============================================================================
// Task 7.1: Unit tests for ArtifactParser.Parse and ArtifactSerializer.Serialize
// Requirements: 7.1, 7.2, 7.3, 7.4
// ============================================================================

func TestArtifactParser_ValidOutputAllSections(t *testing.T) {
	raw := `===BRD===
Business Requirements Document content here.
===END_BRD===

===SRS===
Software Requirements Specification content here.
===END_SRS===

===NFR===
Non-Functional Requirements content here.
===END_NFR===

===GHERKIN===
### Story: User Login
Given a registered user
When they enter valid credentials
Then they are logged in

### Story: User Logout
Given a logged-in user
When they click logout
Then they are logged out
===END_GHERKIN===`

	parser := &artifactCodec{}
	artifacts, err := parser.Parse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have 4 artifacts: BRD, SRS, NFR, and 2 Gherkin stories
	if len(artifacts) != 5 {
		t.Fatalf("expected 5 artifacts, got %d", len(artifacts))
	}

	// Check BRD
	if artifacts[0].Type != ArtifactBRD {
		t.Errorf("expected type %q, got %q", ArtifactBRD, artifacts[0].Type)
	}
	if !strings.Contains(artifacts[0].Content, "Business Requirements Document") {
		t.Errorf("BRD content missing expected text")
	}

	// Check SRS
	if artifacts[1].Type != ArtifactSRS {
		t.Errorf("expected type %q, got %q", ArtifactSRS, artifacts[1].Type)
	}

	// Check NFR
	if artifacts[2].Type != ArtifactNFR {
		t.Errorf("expected type %q, got %q", ArtifactNFR, artifacts[2].Type)
	}

	// Check Gherkin stories
	if artifacts[3].Type != ArtifactGherkin {
		t.Errorf("expected type %q, got %q", ArtifactGherkin, artifacts[3].Type)
	}
	if artifacts[3].Title != "User Login" {
		t.Errorf("expected title %q, got %q", "User Login", artifacts[3].Title)
	}
	if !strings.Contains(artifacts[3].Content, "Given a registered user") {
		t.Errorf("first Gherkin story missing expected content")
	}

	if artifacts[4].Type != ArtifactGherkin {
		t.Errorf("expected type %q, got %q", ArtifactGherkin, artifacts[4].Type)
	}
	if artifacts[4].Title != "User Logout" {
		t.Errorf("expected title %q, got %q", "User Logout", artifacts[4].Title)
	}
}

func TestArtifactParser_MultipleGherkinStories(t *testing.T) {
	raw := `===BRD===
BRD content
===END_BRD===

===SRS===
SRS content
===END_SRS===

===NFR===
NFR content
===END_NFR===

===GHERKIN===
### Story: First Story
Given precondition A
When action A
Then result A

### Story: Second Story
Given precondition B
When action B
Then result B

### Story: Third Story
Given precondition C
When action C
Then result C
===END_GHERKIN===`

	parser := &artifactCodec{}
	artifacts, err := parser.Parse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// BRD + SRS + NFR + 3 Gherkin = 6
	if len(artifacts) != 6 {
		t.Fatalf("expected 6 artifacts, got %d", len(artifacts))
	}

	// Verify all three Gherkin stories
	gherkinArtifacts := artifacts[3:]
	titles := []string{"First Story", "Second Story", "Third Story"}
	for i, a := range gherkinArtifacts {
		if a.Type != ArtifactGherkin {
			t.Errorf("artifact[%d]: expected type %q, got %q", i+3, ArtifactGherkin, a.Type)
		}
		if a.Title != titles[i] {
			t.Errorf("artifact[%d]: expected title %q, got %q", i+3, titles[i], a.Title)
		}
	}
}

func TestArtifactParser_MalformedMissingDelimiter(t *testing.T) {
	tests := []struct {
		name            string
		raw             string
		expectedSection string
	}{
		{
			name: "missing BRD end delimiter",
			raw: `===BRD===
BRD content without end delimiter

===SRS===
SRS content
===END_SRS===

===NFR===
NFR content
===END_NFR===

===GHERKIN===
### Story: Test
Given x
When y
Then z
===END_GHERKIN===`,
			expectedSection: "BRD",
		},
		{
			name: "missing SRS start delimiter",
			raw: `===BRD===
BRD content
===END_BRD===

SRS content without start delimiter
===END_SRS===

===NFR===
NFR content
===END_NFR===

===GHERKIN===
### Story: Test
Given x
When y
Then z
===END_GHERKIN===`,
			expectedSection: "SRS",
		},
		{
			name: "missing GHERKIN end delimiter",
			raw: `===BRD===
BRD content
===END_BRD===

===SRS===
SRS content
===END_SRS===

===NFR===
NFR content
===END_NFR===

===GHERKIN===
### Story: Test
Given x
When y
Then z`,
			expectedSection: "GHERKIN",
		},
		{
			name: "missing NFR section entirely",
			raw: `===BRD===
BRD content
===END_BRD===

===SRS===
SRS content
===END_SRS===

===GHERKIN===
### Story: Test
Given x
When y
Then z
===END_GHERKIN===`,
			expectedSection: "NFR",
		},
	}

	parser := &artifactCodec{}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parser.Parse(tc.raw)
			if err == nil {
				t.Fatal("expected error for malformed input, got nil")
			}

			var pe *ParseError
			if !errors.As(err, &pe) {
				t.Fatalf("expected ParseError, got %T: %v", err, err)
			}
			if pe.Section != tc.expectedSection {
				t.Errorf("expected section %q, got %q", tc.expectedSection, pe.Section)
			}
		})
	}
}

func TestArtifactSerializer_RoundTrip(t *testing.T) {
	original := []Artifact{
		{Type: ArtifactBRD, Title: "BRD", Content: "Business requirements content."},
		{Type: ArtifactSRS, Title: "SRS", Content: "Software requirements content."},
		{Type: ArtifactNFR, Title: "NFR", Content: "Non-functional requirements content."},
		{Type: ArtifactGherkin, Title: "User Login", Content: "Given a user\nWhen they login\nThen they see dashboard"},
		{Type: ArtifactGherkin, Title: "User Logout", Content: "Given a logged-in user\nWhen they logout\nThen they see login page"},
	}

	codec := &artifactCodec{}

	serialized, err := codec.Serialize(original)
	if err != nil {
		t.Fatalf("serialize error: %v", err)
	}

	parsed, err := codec.Parse(serialized)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if len(parsed) != len(original) {
		t.Fatalf("expected %d artifacts, got %d", len(original), len(parsed))
	}

	for i := range original {
		if parsed[i].Type != original[i].Type {
			t.Errorf("artifact[%d]: type mismatch: %q vs %q", i, original[i].Type, parsed[i].Type)
		}
		if parsed[i].Title != original[i].Title {
			t.Errorf("artifact[%d]: title mismatch: %q vs %q", i, original[i].Title, parsed[i].Title)
		}
		if strings.TrimSpace(parsed[i].Content) != strings.TrimSpace(original[i].Content) {
			t.Errorf("artifact[%d]: content mismatch:\n  original: %q\n  parsed:   %q", i, original[i].Content, parsed[i].Content)
		}
	}
}


// ============================================================================
// Task 7.2: Property test — Artifact parsing round-trip
// Feature: elicitation-engine, Property 6: Artifact parsing round-trip
// Validates: Requirements 7.1, 7.2, 7.4
// ============================================================================

func TestArtifactParseRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate random content for BRD, SRS, NFR (no delimiters in content)
		// Use word-based generators to avoid content that is only whitespace/newlines
		brdContent := rapid.StringMatching(`[a-zA-Z0-9][a-zA-Z0-9 .,;:!?]{9,199}`).Draw(t, "brdContent")
		srsContent := rapid.StringMatching(`[a-zA-Z0-9][a-zA-Z0-9 .,;:!?]{9,199}`).Draw(t, "srsContent")
		nfrContent := rapid.StringMatching(`[a-zA-Z0-9][a-zA-Z0-9 .,;:!?]{9,199}`).Draw(t, "nfrContent")

		// Generate 1-5 Gherkin stories
		numStories := rapid.IntRange(1, 5).Draw(t, "numStories")
		gherkinArtifacts := make([]Artifact, numStories)
		for i := 0; i < numStories; i++ {
			// Titles must not have trailing/leading spaces (they get trimmed on parse)
			title := rapid.StringMatching(`[A-Z][a-zA-Z0-9]{2,29}[a-zA-Z0-9]`).Draw(t, fmt.Sprintf("storyTitle_%d", i))
			given := rapid.StringMatching(`[a-zA-Z0-9][a-zA-Z0-9 ]{3,48}[a-zA-Z0-9]`).Draw(t, fmt.Sprintf("given_%d", i))
			when := rapid.StringMatching(`[a-zA-Z0-9][a-zA-Z0-9 ]{3,48}[a-zA-Z0-9]`).Draw(t, fmt.Sprintf("when_%d", i))
			then := rapid.StringMatching(`[a-zA-Z0-9][a-zA-Z0-9 ]{3,48}[a-zA-Z0-9]`).Draw(t, fmt.Sprintf("then_%d", i))
			gherkinArtifacts[i] = Artifact{
				Type:    ArtifactGherkin,
				Title:   title,
				Content: fmt.Sprintf("Given %s\nWhen %s\nThen %s", given, when, then),
			}
		}

		original := []Artifact{
			{Type: ArtifactBRD, Title: "BRD", Content: brdContent},
			{Type: ArtifactSRS, Title: "SRS", Content: srsContent},
			{Type: ArtifactNFR, Title: "NFR", Content: nfrContent},
		}
		original = append(original, gherkinArtifacts...)

		codec := &artifactCodec{}

		// Serialize
		serialized, err := codec.Serialize(original)
		if err != nil {
			t.Fatalf("serialize error: %v", err)
		}

		// Parse back
		parsed, err := codec.Parse(serialized)
		if err != nil {
			t.Fatalf("parse error: %v", err)
		}

		// Property: same number of artifacts
		if len(parsed) != len(original) {
			t.Fatalf("expected %d artifacts, got %d", len(original), len(parsed))
		}

		// Property: each artifact matches type, title, and content
		for i := range original {
			if parsed[i].Type != original[i].Type {
				t.Errorf("artifact[%d]: type mismatch: %q vs %q", i, original[i].Type, parsed[i].Type)
			}
			if parsed[i].Title != original[i].Title {
				t.Errorf("artifact[%d]: title mismatch: %q vs %q", i, original[i].Title, parsed[i].Title)
			}
			if strings.TrimSpace(parsed[i].Content) != strings.TrimSpace(original[i].Content) {
				t.Errorf("artifact[%d]: content mismatch:\n  original: %q\n  parsed:   %q", i, original[i].Content, parsed[i].Content)
			}
		}
	})
}

// ============================================================================
// Task 7.3: Property test — Malformed synthesis error identification
// Feature: elicitation-engine, Property 7: Malformed synthesis error identification
// Validates: Requirements 7.3
// ============================================================================

func TestMalformedSynthesisError(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate valid content for all sections
		brdContent := rapid.StringMatching(`[a-zA-Z0-9 ]{10,50}`).Draw(t, "brdContent")
		srsContent := rapid.StringMatching(`[a-zA-Z0-9 ]{10,50}`).Draw(t, "srsContent")
		nfrContent := rapid.StringMatching(`[a-zA-Z0-9 ]{10,50}`).Draw(t, "nfrContent")
		storyTitle := rapid.StringMatching(`[A-Z][a-zA-Z0-9 ]{3,20}`).Draw(t, "storyTitle")
		given := rapid.StringMatching(`[a-zA-Z0-9 ]{5,30}`).Draw(t, "given")
		when := rapid.StringMatching(`[a-zA-Z0-9 ]{5,30}`).Draw(t, "when")
		then := rapid.StringMatching(`[a-zA-Z0-9 ]{5,30}`).Draw(t, "then")

		// Build valid synthesis output as sections
		sections := []struct {
			typeName string
			content  string
		}{
			{"BRD", brdContent},
			{"SRS", srsContent},
			{"NFR", nfrContent},
			{"GHERKIN", fmt.Sprintf("### Story: %s\nGiven %s\nWhen %s\nThen %s", storyTitle, given, when, then)},
		}

		// Pick one section to corrupt
		corruptIdx := rapid.IntRange(0, len(sections)-1).Draw(t, "corruptIdx")
		corruptedType := sections[corruptIdx].typeName

		// Build the raw output, corrupting exactly one section's delimiter
		var sb strings.Builder
		for i, sec := range sections {
			if i == corruptIdx {
				// Corrupt the start delimiter by mangling it
				sb.WriteString(fmt.Sprintf("===%s_BROKEN===\n", sec.typeName))
				sb.WriteString(sec.content)
				sb.WriteString(fmt.Sprintf("\n===END_%s===\n\n", sec.typeName))
			} else {
				sb.WriteString(fmt.Sprintf("===%s===\n", sec.typeName))
				sb.WriteString(sec.content)
				sb.WriteString(fmt.Sprintf("\n===END_%s===\n\n", sec.typeName))
			}
		}

		parser := &artifactCodec{}
		_, err := parser.Parse(sb.String())

		// Property: error must be non-nil
		if err == nil {
			t.Fatal("expected error for corrupted synthesis output, got nil")
		}

		// Property: error identifies the corrupted section by type name
		var pe *ParseError
		if !errors.As(err, &pe) {
			t.Fatalf("expected ParseError, got %T: %v", err, err)
		}
		if pe.Section != corruptedType {
			t.Errorf("expected section %q in error, got %q", corruptedType, pe.Section)
		}

		// Property: error message contains the section type name
		if !strings.Contains(err.Error(), corruptedType) {
			t.Errorf("error message should contain %q, got: %v", corruptedType, err)
		}
	})
}
