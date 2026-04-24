package elicitation

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ============================================================================
// Task 4.1: Unit tests for PersonaLoader
// Requirements: 2.1, 2.3, 10.1, 10.2, 10.3, 10.4
// ============================================================================

// setupTestPrompts creates temporary persona template files for testing.
func setupTestPrompts(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	socratic := `You are a Socratic Business Analyst. Your role is to use guided question techniques
to help users discover hidden assumptions, uncover stakeholder needs, and define success criteria.
Ask probing questions about edge cases and dependencies. Maintain a collaborative dialogue.
Never provide direct answers — instead, guide the user to discover insights through careful questioning.
Focus on understanding the business context, identifying all stakeholders, and ensuring
requirements are complete and unambiguous.`

	hostile := `You are a Hostile Systems Architect. Your role is to aggressively challenge every proposal.
Probe for architectural weakness and failure modes. Identify scalability gaps and missing
non-functional requirements. Question integration risks and security vulnerabilities.
Do not accept vague answers — demand specifics on performance, reliability, and disaster recovery.
Your goal is to break the proposal before it reaches production.`

	advisor := `You are a Trusted Advisor. Your role is to gently fill in ambiguities and question
decisions in a supportive, non-confrontational way. Offer gentle suggestions and ask clarifying
questions that help refine both product requirements and architectural concerns.
Be encouraging and supportive while ensuring nothing important is overlooked.
Help the user think through trade-offs without being prescriptive.`

	for name, content := range map[string]string{
		"socratic_ba.tmpl":     socratic,
		"hostile_sa.tmpl":      hostile,
		"trusted_advisor.tmpl": advisor,
	} {
		err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644)
		if err != nil {
			t.Fatalf("failed to write test prompt %s: %v", name, err)
		}
	}
	return dir
}

func TestPersonaLoader_LoadSocraticBA(t *testing.T) {
	dir := setupTestPrompts(t)
	loader := NewFilePersonaLoader(dir)

	prompt, err := loader.Load(PersonaSocraticBA)
	if err != nil {
		t.Fatalf("unexpected error loading socratic BA: %v", err)
	}
	if prompt == "" {
		t.Fatal("expected non-empty prompt for socratic BA")
	}
}

func TestPersonaLoader_LoadHostileSA(t *testing.T) {
	dir := setupTestPrompts(t)
	loader := NewFilePersonaLoader(dir)

	prompt, err := loader.Load(PersonaHostileSA)
	if err != nil {
		t.Fatalf("unexpected error loading hostile SA: %v", err)
	}
	if prompt == "" {
		t.Fatal("expected non-empty prompt for hostile SA")
	}
}

func TestPersonaLoader_LoadTrustedAdvisor(t *testing.T) {
	dir := setupTestPrompts(t)
	loader := NewFilePersonaLoader(dir)

	prompt, err := loader.Load(PersonaTrustedAdv)
	if err != nil {
		t.Fatalf("unexpected error loading trusted advisor: %v", err)
	}
	if prompt == "" {
		t.Fatal("expected non-empty prompt for trusted advisor")
	}
}

func TestPersonaLoader_InvalidPersonaReturnsError(t *testing.T) {
	dir := setupTestPrompts(t)
	loader := NewFilePersonaLoader(dir)

	_, err := loader.Load(PersonaType("nonexistent_persona"))
	if err == nil {
		t.Fatal("expected error for invalid persona, got nil")
	}
	if !errors.Is(err, ErrInvalidPersona) {
		t.Fatalf("expected ErrInvalidPersona, got: %v", err)
	}
}

func TestPersonaLoader_SocraticBAContainsExpectedKeywords(t *testing.T) {
	dir := setupTestPrompts(t)
	loader := NewFilePersonaLoader(dir)

	prompt, err := loader.Load(PersonaSocraticBA)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lower := strings.ToLower(prompt)
	for _, keyword := range []string{"question", "assumption", "stakeholder"} {
		if !strings.Contains(lower, keyword) {
			t.Errorf("socratic BA prompt missing expected keyword %q", keyword)
		}
	}
}

func TestPersonaLoader_HostileSAContainsExpectedKeywords(t *testing.T) {
	dir := setupTestPrompts(t)
	loader := NewFilePersonaLoader(dir)

	prompt, err := loader.Load(PersonaHostileSA)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lower := strings.ToLower(prompt)
	for _, keyword := range []string{"challenge", "failure"} {
		if !strings.Contains(lower, keyword) {
			t.Errorf("hostile SA prompt missing expected keyword %q", keyword)
		}
	}
	// Check for "scalability" or "weakness"
	if !strings.Contains(lower, "scalability") && !strings.Contains(lower, "weakness") {
		t.Error("hostile SA prompt missing expected keyword 'scalability' or 'weakness'")
	}
}

func TestPersonaLoader_TrustedAdvisorContainsExpectedKeywords(t *testing.T) {
	dir := setupTestPrompts(t)
	loader := NewFilePersonaLoader(dir)

	prompt, err := loader.Load(PersonaTrustedAdv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lower := strings.ToLower(prompt)
	// Check for "ambiguity" or "ambiguities"
	if !strings.Contains(lower, "ambiguity") && !strings.Contains(lower, "ambiguities") {
		t.Error("trusted advisor prompt missing expected keyword 'ambiguity' or 'ambiguities'")
	}
	// Check for "gentle" or "supportive"
	if !strings.Contains(lower, "gentle") && !strings.Contains(lower, "supportive") {
		t.Error("trusted advisor prompt missing expected keyword 'gentle' or 'supportive'")
	}
}

func TestPersonaLoader_MissingFileReturnsError(t *testing.T) {
	// Point to an empty directory — no template files exist
	dir := t.TempDir()
	loader := NewFilePersonaLoader(dir)

	_, err := loader.Load(PersonaSocraticBA)
	if err == nil {
		t.Fatal("expected error when template file is missing, got nil")
	}
}
