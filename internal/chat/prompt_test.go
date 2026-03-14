// Package chat — Tests for BuildSystemPrompt (IFS protocol + companion definition).
package chat

import (
	"strings"
	"testing"
)

// ── IFS Protocol presence ──────────────────────────────────────────────────────

// TestBuildSystemPromptContainsIFSProtocolMarkers verifies that the assembled
// prompt includes the non-editable IFS protocol section markers.
func TestBuildSystemPromptContainsIFSProtocolMarkers(t *testing.T) {
	prompt := BuildSystemPrompt("Kira", "", nil, "")

	if !strings.Contains(prompt, "[IFS PROTOCOL — DO NOT MODIFY]") {
		t.Error("expected prompt to contain IFS protocol opening marker")
	}
	if !strings.Contains(prompt, "[END IFS PROTOCOL]") {
		t.Error("expected prompt to contain IFS protocol closing marker")
	}
}

// TestBuildSystemPromptContainsCompanionDefinitionMarkers verifies that the
// companion definition section markers are present.
func TestBuildSystemPromptContainsCompanionDefinitionMarkers(t *testing.T) {
	prompt := BuildSystemPrompt("Kira", "", nil, "")

	if !strings.Contains(prompt, "[COMPANION DEFINITION]") {
		t.Error("expected prompt to contain companion definition opening marker")
	}
	if !strings.Contains(prompt, "[END COMPANION DEFINITION]") {
		t.Error("expected prompt to contain companion definition closing marker")
	}
}

// TestBuildSystemPromptProtocolBeforeDefinition verifies that the IFS protocol
// section appears before the companion definition section.
func TestBuildSystemPromptProtocolBeforeDefinition(t *testing.T) {
	prompt := BuildSystemPrompt("Kira", "", nil, "")

	protocolIdx := strings.Index(prompt, "[IFS PROTOCOL — DO NOT MODIFY]")
	definitionIdx := strings.Index(prompt, "[COMPANION DEFINITION]")

	if protocolIdx < 0 {
		t.Fatal("IFS protocol marker not found")
	}
	if definitionIdx < 0 {
		t.Fatal("companion definition marker not found")
	}
	if protocolIdx >= definitionIdx {
		t.Error("expected IFS protocol to appear before companion definition")
	}
}

// ── Core IFS knowledge presence ───────────────────────────────────────────────

// TestBuildSystemPromptContains6Fs verifies that the 6 F's are present in the
// assembled prompt.
func TestBuildSystemPromptContains6Fs(t *testing.T) {
	prompt := BuildSystemPrompt("Kira", "", nil, "")

	sixFs := []string{"FIND", "FOCUS", "FLESH OUT", "FEEL TOWARD", "BEFRIEND", "FEAR"}
	for _, f := range sixFs {
		if !strings.Contains(prompt, f) {
			t.Errorf("expected prompt to contain 6 F's step %q", f)
		}
	}
}

// TestBuildSystemPromptContainsPartsTaxonomy verifies that all three part types
// are described in the prompt.
func TestBuildSystemPromptContainsPartsTaxonomy(t *testing.T) {
	prompt := BuildSystemPrompt("Kira", "", nil, "")

	parts := []string{"MANAGERS", "FIREFIGHTERS", "EXILES"}
	for _, p := range parts {
		if !strings.Contains(prompt, p) {
			t.Errorf("expected prompt to contain parts taxonomy entry %q", p)
		}
	}
}

// TestBuildSystemPromptContainsSelfEnergy verifies that the 8 C's of Self-energy
// are present in the prompt.
func TestBuildSystemPromptContainsSelfEnergy(t *testing.T) {
	prompt := BuildSystemPrompt("Kira", "", nil, "")

	eightCs := []string{
		"Calm", "Curiosity", "Compassion", "Clarity",
		"Confidence", "Courage", "Creativity", "Connectedness",
	}
	for _, c := range eightCs {
		if !strings.Contains(prompt, c) {
			t.Errorf("expected prompt to contain 8 C's quality %q", c)
		}
	}
}

// TestBuildSystemPromptContains5Ps verifies that the 5 P's of Self are present.
func TestBuildSystemPromptContains5Ps(t *testing.T) {
	prompt := BuildSystemPrompt("Kira", "", nil, "")

	fivePs := []string{"Presence", "Perspective", "Patience", "Persistence", "Playfulness"}
	for _, p := range fivePs {
		if !strings.Contains(prompt, p) {
			t.Errorf("expected prompt to contain 5 P's quality %q", p)
		}
	}
}

// TestBuildSystemPromptContainsMillionDollarQuestion verifies that the "million
// dollar question" is present — it is the critical Self-energy check.
func TestBuildSystemPromptContainsMillionDollarQuestion(t *testing.T) {
	prompt := BuildSystemPrompt("Kira", "", nil, "")

	if !strings.Contains(prompt, "How do you feel toward this part") {
		t.Error("expected prompt to contain the million dollar question")
	}
}

// TestBuildSystemPromptContainsCrisisGuidance verifies that crisis safety
// language is present in the prompt.
func TestBuildSystemPromptContainsCrisisGuidance(t *testing.T) {
	prompt := BuildSystemPrompt("Kira", "", nil, "")

	if !strings.Contains(prompt, "988") {
		t.Error("expected prompt to contain US crisis line number 988")
	}
	if !strings.Contains(prompt, "NOT a therapist") {
		t.Error("expected prompt to disclaim therapy role")
	}
}

// TestBuildSystemPromptContainsNotTherapist verifies the therapy disclaimer.
func TestBuildSystemPromptContainsNotTherapist(t *testing.T) {
	prompt := BuildSystemPrompt("Kira", "", nil, "")

	if !strings.Contains(prompt, "NOT a therapist") {
		t.Error("expected prompt to contain therapy disclaimer")
	}
}

// ── Companion name interpolation ──────────────────────────────────────────────

// TestBuildSystemPromptCompanionName verifies that the companion name appears
// in the companion definition section.
func TestBuildSystemPromptCompanionName(t *testing.T) {
	prompt := BuildSystemPrompt("Kira", "", nil, "")

	if !strings.Contains(prompt, "Your name is: Kira") {
		t.Error("expected prompt to contain companion name 'Kira'")
	}
}

// TestBuildSystemPromptCompanionNameCustom verifies that a custom companion name
// is correctly interpolated.
func TestBuildSystemPromptCompanionNameCustom(t *testing.T) {
	prompt := BuildSystemPrompt("Aria", "", nil, "")

	if !strings.Contains(prompt, "Your name is: Aria") {
		t.Errorf("expected prompt to contain companion name 'Aria'")
	}
	// The old default name must not appear.
	if strings.Contains(prompt, "Your name is: Kira") {
		t.Error("expected old companion name 'Kira' not to appear")
	}
}

// ── User name interpolation ───────────────────────────────────────────────────

// TestBuildSystemPromptUserName verifies that the user name appears when set.
func TestBuildSystemPromptUserName(t *testing.T) {
	prompt := BuildSystemPrompt("Kira", "Alex", nil, "")

	if !strings.Contains(prompt, "The user's name is: Alex") {
		t.Error("expected prompt to contain user name 'Alex'")
	}
}

// TestBuildSystemPromptEmptyUserName verifies that when no user name is set,
// the prompt instructs the companion to use "friend" as a fallback.
func TestBuildSystemPromptEmptyUserName(t *testing.T) {
	prompt := BuildSystemPrompt("Kira", "", nil, "")

	if !strings.Contains(prompt, "friend") {
		t.Error("expected prompt to contain 'friend' fallback when user name is empty")
	}
	// The explicit "user's name is" line must not appear with an empty name.
	if strings.Contains(prompt, "The user's name is: \n") {
		t.Error("expected no empty user name line")
	}
}

// ── Focus areas ───────────────────────────────────────────────────────────────

// TestBuildSystemPromptFocusAreas verifies that focus areas appear in the prompt.
func TestBuildSystemPromptFocusAreas(t *testing.T) {
	prompt := BuildSystemPrompt("Kira", "", []string{"anxiety", "perfectionism"}, "")

	if !strings.Contains(prompt, "anxiety") {
		t.Error("expected prompt to contain focus area 'anxiety'")
	}
	if !strings.Contains(prompt, "perfectionism") {
		t.Error("expected prompt to contain focus area 'perfectionism'")
	}
	if !strings.Contains(prompt, "Focus areas:") {
		t.Error("expected prompt to contain 'Focus areas:' label")
	}
}

// TestBuildSystemPromptFocusAreasMultiple verifies that multiple focus areas
// are joined correctly.
func TestBuildSystemPromptFocusAreasMultiple(t *testing.T) {
	areas := []string{"anxiety", "inner critic", "procrastination", "abandonment"}
	prompt := BuildSystemPrompt("Kira", "", areas, "")

	for _, area := range areas {
		if !strings.Contains(prompt, area) {
			t.Errorf("expected prompt to contain focus area %q", area)
		}
	}
}

// TestBuildSystemPromptNoFocusAreas verifies that when no focus areas are set,
// the "Focus areas:" label does not appear.
func TestBuildSystemPromptNoFocusAreas(t *testing.T) {
	prompt := BuildSystemPrompt("Kira", "", nil, "")

	if strings.Contains(prompt, "Focus areas:") {
		t.Error("expected no 'Focus areas:' section when focus areas are empty")
	}
}

// TestBuildSystemPromptEmptyFocusAreas verifies that an empty slice behaves the
// same as nil — no focus areas section.
func TestBuildSystemPromptEmptyFocusAreas(t *testing.T) {
	prompt := BuildSystemPrompt("Kira", "", []string{}, "")

	if strings.Contains(prompt, "Focus areas:") {
		t.Error("expected no 'Focus areas:' section when focus areas slice is empty")
	}
}

// ── Custom instructions ───────────────────────────────────────────────────────

// TestBuildSystemPromptCustomInstructions verifies that custom instructions
// appear in the prompt.
func TestBuildSystemPromptCustomInstructions(t *testing.T) {
	prompt := BuildSystemPrompt("Kira", "", nil, "Speak in a gentle, poetic tone.")

	if !strings.Contains(prompt, "Speak in a gentle, poetic tone.") {
		t.Error("expected prompt to contain custom instructions")
	}
}

// TestBuildSystemPromptNoCustomInstructions verifies that when no custom
// instructions are set, the additional instructions section does not appear.
func TestBuildSystemPromptNoCustomInstructions(t *testing.T) {
	prompt := BuildSystemPrompt("Kira", "", nil, "")

	if strings.Contains(prompt, "Additional instructions from the user:") {
		t.Error("expected no custom instructions section when empty")
	}
}

// ── Minimal config ────────────────────────────────────────────────────────────

// TestBuildSystemPromptMinimalConfig verifies that a minimal config (name only)
// still produces a valid, complete prompt with the full IFS protocol.
func TestBuildSystemPromptMinimalConfig(t *testing.T) {
	prompt := BuildSystemPrompt("Companion", "", nil, "")

	// Must have the IFS protocol.
	if !strings.Contains(prompt, "[IFS PROTOCOL — DO NOT MODIFY]") {
		t.Error("expected minimal prompt to contain IFS protocol")
	}
	// Must have the companion name.
	if !strings.Contains(prompt, "Your name is: Companion") {
		t.Error("expected minimal prompt to contain companion name")
	}
	// Must NOT have focus areas or custom instructions sections.
	if strings.Contains(prompt, "Focus areas:") {
		t.Error("expected no focus areas section in minimal prompt")
	}
	if strings.Contains(prompt, "Additional instructions") {
		t.Error("expected no custom instructions section in minimal prompt")
	}
}

// TestBuildSystemPromptFullConfig verifies that a fully populated config
// produces a prompt containing all sections.
func TestBuildSystemPromptFullConfig(t *testing.T) {
	prompt := BuildSystemPrompt(
		"Aria",
		"Alex",
		[]string{"anxiety", "perfectionism", "inner critic"},
		"Always end sessions with a grounding exercise.",
	)

	checks := []struct {
		name    string
		contain string
	}{
		{"IFS protocol marker", "[IFS PROTOCOL — DO NOT MODIFY]"},
		{"companion name", "Your name is: Aria"},
		{"user name", "The user's name is: Alex"},
		{"focus area anxiety", "anxiety"},
		{"focus area perfectionism", "perfectionism"},
		{"focus area inner critic", "inner critic"},
		{"custom instructions", "Always end sessions with a grounding exercise."},
		{"companion definition end", "[END COMPANION DEFINITION]"},
	}

	for _, c := range checks {
		if !strings.Contains(prompt, c.contain) {
			t.Errorf("full config test: expected prompt to contain %s (%q)", c.name, c.contain)
		}
	}
}

// ── IFSProtocol constant ──────────────────────────────────────────────────────

// TestIFSProtocolConstantNotEmpty verifies that the IFSProtocol constant is
// non-empty and of substantial length (the prompt is ~3000+ words).
func TestIFSProtocolConstantNotEmpty(t *testing.T) {
	if len(IFSProtocol) == 0 {
		t.Fatal("IFSProtocol constant must not be empty")
	}
	// The prompt should be substantial — at least 10,000 characters.
	if len(IFSProtocol) < 10000 {
		t.Errorf("IFSProtocol constant seems too short (%d chars) — expected at least 10000", len(IFSProtocol))
	}
}

// TestIFSProtocolConstantContainsNoBadParts verifies the "no bad parts"
// principle is present — it is foundational to IFS.
func TestIFSProtocolConstantContainsNoBadParts(t *testing.T) {
	if !strings.Contains(IFSProtocol, "No bad parts") {
		t.Error("expected IFSProtocol to contain 'No bad parts' principle")
	}
}

// TestIFSProtocolConstantContainsUnblending verifies that unblending techniques
// are present — they are essential for practical IFS work.
func TestIFSProtocolConstantContainsUnblending(t *testing.T) {
	if !strings.Contains(IFSProtocol, "Blending") && !strings.Contains(IFSProtocol, "blending") {
		t.Error("expected IFSProtocol to contain unblending guidance")
	}
}

// TestIFSProtocolConstantContainsExileWarning verifies that the exile work
// safety warning is present.
func TestIFSProtocolConstantContainsExileWarning(t *testing.T) {
	if !strings.Contains(IFSProtocol, "NEVER rush to exile work") {
		t.Error("expected IFSProtocol to contain exile work safety warning")
	}
}
