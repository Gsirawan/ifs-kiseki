// Package crisis implements crisis keyword detection and resource display.
// This is NON-NEGOTIABLE — ships in V1 or the product doesn't ship.
package crisis

import (
	"regexp"
	"strings"
)

// ── Pattern definitions ──────────────────────────────────────────
//
// Each entry is a raw regex string. Patterns are compiled ONCE in
// NewRegexCrisisDetector — never per-scan.
//
// Design principles:
//   - Word boundaries (\b) prevent "killed it at work" from matching "kill"
//   - Phrase patterns (multi-word) are matched literally with flexible spacing
//   - Case-insensitive flag applied at compile time ((?i) prefix)
//   - Better to show resources unnecessarily than miss a real crisis

type patternGroup struct {
	category string
	patterns []string
}

var patternGroups = []patternGroup{
	{
		category: "suicidal ideation",
		patterns: []string{
			// "kill myself" and gerund "killing myself"
			`(?i)\bkill(?:ing)?\s+myself\b`,
			`(?i)\bend\s+my\s+life\b`,
			`(?i)\bwant(?:ed|ing)?\s+to\s+die\b`,
			`(?i)\bsuicid(?:e|al|ally)\b`,
			`(?i)\bdon'?t\s+want\s+to\s+be\s+here\b`,
			`(?i)\bno\s+reason\s+to\s+live\b`,
			`(?i)\bbetter\s+off\s+dead\b`,
			// "end it all" and "ending it all"
			`(?i)\bend(?:ing)?\s+it\s+all\b`,
			// "can't go on" and "cannot go on"
			`(?i)\bcan(?:not|'?t)\s+go\s+on\b`,
			`(?i)\bnot\s+worth\s+living\b`,
			`(?i)\blife\s+is\s+not\s+worth\s+living\b`,
			`(?i)\bwish\s+I\s+w(?:as|ere)\s+dead\b`,
		},
	},
	{
		category: "self-harm",
		patterns: []string{
			`(?i)\bhurt(?:ing)?\s+myself\b`,
			`(?i)\bcut(?:ting)?\s+myself\b`,
			`(?i)\bself[- ]harm(?:ing)?\b`,
			`(?i)\bburn(?:ing)?\s+myself\b`,
			`(?i)\bhit(?:ting)?\s+myself\b`,
			`(?i)\binjur(?:e|ing)\s+myself\b`,
		},
	},
	{
		category: "harm to others",
		patterns: []string{
			// Match "kill/hurt/harm" followed by a specific person pronoun or
			// "someone/somebody". Exclude "it", "my [body part]", possessives.
			// Use a negative lookahead for possessive 's and for "it".
			// Pattern: verb + whitespace + (pronoun/someone) + word boundary
			// but NOT followed by "'s" (possessive) or "self" (reflexive — handled above).
			`(?i)\bkill\s+(?:someone|somebody|him|her|them|you)(?:'s)?\b`,
			`(?i)\bhurt\s+(?:someone|somebody|him|her|them|you)(?:'s)?\b`,
			`(?i)\bharm\s+(?:someone|somebody|him|her|them|you)(?:'s)?\b`,
			// "want to hurt/kill" — intent phrases (no object required)
			`(?i)\bwant\s+to\s+(?:hurt|kill|harm)\s+(?:someone|somebody|him|her|them|you)\b`,
			`(?i)\bgoing\s+to\s+(?:hurt|kill|harm)\s+(?:someone|somebody|him|her|them|you)\b`,
		},
	},
	{
		category: "acute crisis",
		patterns: []string{
			`(?i)\boverdos(?:e|ing|ed)\b`,
			`(?i)\btook\s+(?:too\s+many\s+)?pills\b`,
			`(?i)\btaking\s+(?:too\s+many\s+)?pills\b`,
			`(?i)\bswallowed\s+pills\b`,
		},
	},
}

// ── RegexCrisisDetector ──────────────────────────────────────────

// RegexCrisisDetector implements the Detector interface using pre-compiled
// regular expressions. Patterns are compiled once at construction time.
type RegexCrisisDetector struct {
	compiled []compiledGroup
	country  string
}

type compiledGroup struct {
	category string
	regexps  []*regexp.Regexp
}

// NewRegexCrisisDetector creates a RegexCrisisDetector for the given country
// code (used to select crisis resources). Panics if any pattern fails to
// compile — this is a programming error, not a runtime error.
func NewRegexCrisisDetector(country string) *RegexCrisisDetector {
	if country == "" {
		country = "US"
	}

	compiled := make([]compiledGroup, 0, len(patternGroups))
	for _, pg := range patternGroups {
		cg := compiledGroup{category: pg.category}
		for _, pat := range pg.patterns {
			re := regexp.MustCompile(pat)
			cg.regexps = append(cg.regexps, re)
		}
		compiled = append(compiled, cg)
	}

	return &RegexCrisisDetector{
		compiled: compiled,
		country:  strings.ToUpper(country),
	}
}

// Scan checks text for crisis indicators.
// Returns (true, category) if a pattern matches, (false, "") otherwise.
//
// Privacy: this method does NOT log the message content. The caller
// must not log content either — only the category is safe to log.
func (d *RegexCrisisDetector) Scan(text string) (detected bool, category string) {
	for _, cg := range d.compiled {
		for _, re := range cg.regexps {
			if re.MatchString(text) {
				return true, cg.category
			}
		}
	}
	return false, ""
}

// Resources returns the crisis resource text for this detector's country.
func (d *RegexCrisisDetector) Resources() string {
	return Resources(d.country)
}
