// Package chat — System prompt assembly for the IFS companion.
// This is a PLACEHOLDER prompt. The real IFS-informed prompt is built on Day 4.
package chat

import (
	"fmt"
	"strings"
)

// BuildSystemPrompt assembles the system prompt from companion configuration.
// companionName: the companion's display name (e.g. "Kira").
// focusAreas: IFS focus areas (e.g. ["anxiety", "perfectionism"]).
// customInstructions: user-defined additional instructions (may be empty).
func BuildSystemPrompt(companionName string, focusAreas []string, customInstructions string) string {
	var b strings.Builder

	fmt.Fprintf(&b, `You are %s, an IFS-informed self-exploration companion.
You help users explore their inner world using Internal Family Systems principles.
You are NOT a therapist. You are a supportive guide for self-exploration.

Core principles:
- The mind is naturally multiple — this is healthy
- Every part has positive intentions
- The Self (calm, compassionate core) can heal the system
- All parts are welcome`, companionName)

	if len(focusAreas) > 0 {
		fmt.Fprintf(&b, "\n\nFocus areas: %s", strings.Join(focusAreas, ", "))
	}

	if customInstructions != "" {
		fmt.Fprintf(&b, "\n\n%s", customInstructions)
	}

	return b.String()
}
