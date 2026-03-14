// Package chat — System prompt assembly for the IFS companion.
//
// The system prompt is assembled in two sections:
//
//  1. IFS Protocol (non-editable) — the IFSProtocol constant from prompt_ifs.go.
//     This encodes deep IFS knowledge: the 6 F's, parts taxonomy, Self-energy,
//     session flow, unblending techniques, exile work, and critical safety rules.
//
//  2. Companion Definition (user-editable) — name, user name, focus areas, and
//     custom instructions drawn from the companion config.
//
// After assembly, InjectMemoryContext may append a [MEMORY CONTEXT] section
// containing relevant fragments from previous sessions.
package chat

import (
	"fmt"
	"strings"

	"github.com/Gsirawan/ifs-kiseki/internal/memory"
)

// BuildSystemPrompt assembles the full system prompt from the IFS protocol
// constant and the companion configuration.
//
// Parameters:
//   - companionName: the companion's display name (e.g. "Kira"). Required.
//   - userName: the user's name. If empty, the companion addresses them as "friend".
//   - focusAreas: IFS focus areas (e.g. ["anxiety", "perfectionism"]). May be nil.
//   - customInstructions: user-defined additional instructions. May be empty.
//
// The returned prompt starts with the full IFS protocol (non-editable) followed
// by the companion definition section (user-editable).
func BuildSystemPrompt(companionName, userName string, focusAreas []string, customInstructions string) string {
	var b strings.Builder

	// Section 1: IFS Protocol — non-editable, sourced from prompt_ifs.go.
	b.WriteString(IFSProtocol)

	// Section 2: Companion Definition — user-editable.
	b.WriteString("\n\n[COMPANION DEFINITION]\n")

	// Companion name — always present.
	fmt.Fprintf(&b, "Your name is: %s\n", companionName)

	// User name — if empty, address the user as "friend".
	if userName != "" {
		fmt.Fprintf(&b, "The user's name is: %s\n", userName)
	} else {
		b.WriteString("The user's name is not set — address them warmly as \"friend\" if you need to use a name.\n")
	}

	// Focus areas — optional. If present, the companion should be especially
	// attentive to these themes when they arise in conversation.
	if len(focusAreas) > 0 {
		fmt.Fprintf(&b, "Focus areas: %s\n", strings.Join(focusAreas, ", "))
		b.WriteString("Be especially attentive when these themes arise — they are areas the user has identified as important to their inner work.\n")
	}

	// Custom instructions — optional. User-defined additions to the companion's
	// behavior. These are appended as-is and take effect after the IFS protocol.
	if customInstructions != "" {
		b.WriteString("\nAdditional instructions from the user:\n")
		b.WriteString(customInstructions)
		b.WriteString("\n")
	}

	b.WriteString("[END COMPANION DEFINITION]")

	return b.String()
}

// maxMemoryTextLen is the maximum character length for a single memory result
// in the injected context block. Longer results are truncated with an ellipsis.
const maxMemoryTextLen = 200

// InjectMemoryContext appends a [MEMORY CONTEXT] section to the system prompt
// containing relevant fragments from previous sessions.
//
// If memories is empty, the prompt is returned unchanged — no section is added.
// Each memory entry is truncated to maxMemoryTextLen characters to keep the
// injected block compact and avoid bloating the system prompt.
func InjectMemoryContext(prompt string, memories []memory.Result) string {
	if len(memories) == 0 {
		return prompt
	}

	var b strings.Builder
	b.WriteString(prompt)
	b.WriteString("\n\n[MEMORY CONTEXT]\nFrom previous sessions:\n")

	for _, m := range memories {
		text := m.Text
		if len(text) > maxMemoryTextLen {
			text = text[:maxMemoryTextLen] + "..."
		}
		timestamp := m.Timestamp.Format("2006-01-02 15:04")
		fmt.Fprintf(&b, "- [%s] %q\n", timestamp, text)
	}

	b.WriteString("[END MEMORY CONTEXT]")
	return b.String()
}
