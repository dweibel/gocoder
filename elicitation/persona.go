package elicitation

import (
	"fmt"
	"os"
	"path/filepath"
)

// PersonaLoader loads persona prompt templates.
type PersonaLoader interface {
	// Load returns the system prompt text for the given persona.
	Load(persona PersonaType) (string, error)
}

// PersonaDisplayNames maps each PersonaType to its human-readable display name.
var PersonaDisplayNames = map[PersonaType]string{
	PersonaSocraticBA: "Socratic Business Analyst",
	PersonaHostileSA:  "Hostile Systems Architect",
	PersonaTrustedAdv: "Trusted Advisor",
}

// DisplayName returns the human-readable name for a PersonaType.
// Returns the raw string value if the persona is not in the map.
func (p PersonaType) DisplayName() string {
	if name, ok := PersonaDisplayNames[p]; ok {
		return name
	}
	return string(p)
}

// personaFileMap maps each PersonaType to its template filename.
var personaFileMap = map[PersonaType]string{
	PersonaSocraticBA: "socratic_ba.tmpl",
	PersonaHostileSA:  "hostile_sa.tmpl",
	PersonaTrustedAdv: "trusted_advisor.tmpl",
}

// filePersonaLoader loads persona prompts from template files on disk.
type filePersonaLoader struct {
	promptsDir string
}

// NewFilePersonaLoader creates a PersonaLoader that reads templates from the
// given directory. Each persona maps to a specific .tmpl file.
func NewFilePersonaLoader(promptsDir string) PersonaLoader {
	return &filePersonaLoader{promptsDir: promptsDir}
}

// Load reads the prompt template file for the given persona and returns its
// content as a string. Returns ErrInvalidPersona for unknown persona types.
func (l *filePersonaLoader) Load(persona PersonaType) (string, error) {
	filename, ok := personaFileMap[persona]
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrInvalidPersona, persona)
	}

	data, err := os.ReadFile(filepath.Join(l.promptsDir, filename))
	if err != nil {
		return "", fmt.Errorf("failed to read persona template %s: %w", filename, err)
	}

	return string(data), nil
}
