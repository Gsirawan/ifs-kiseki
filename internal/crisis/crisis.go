// Package crisis implements crisis keyword detection and resource display.
// This is NON-NEGOTIABLE — ships in V1 or the product doesn't ship.
package crisis

// Detector scans messages for crisis indicators.
type Detector interface {
	// Scan checks a message for crisis keywords/patterns.
	// Returns true if crisis content detected, with the matched pattern.
	Scan(text string) (detected bool, pattern string)

	// Resources returns crisis resource text to display.
	Resources() string
}
