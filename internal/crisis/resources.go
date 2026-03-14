package crisis

import "fmt"

// crisisResource holds the resource data for a single country.
type crisisResource struct {
	// primary is the main crisis line (phone/text).
	primary string
	// secondary is an additional line (text line, emergency, etc.).
	secondary string
	// emergency is the local emergency number.
	emergency string
	// international is the IASP link — always included.
	international string
}

// resourcesByCountry maps ISO 3166-1 alpha-2 country codes to crisis resources.
// US is the default fallback.
var resourcesByCountry = map[string]crisisResource{
	"US": {
		primary:       "988 Suicide & Crisis Lifeline: Call or text 988",
		secondary:     "Crisis Text Line: Text HOME to 741741",
		emergency:     "Emergency Services: Call 911",
		international: "https://www.iasp.info/resources/Crisis_Centres/",
	},
	"GB": {
		primary:       "Samaritans: Call 116 123 (free, 24/7)",
		secondary:     "Crisis Text Line: Text SHOUT to 85258",
		emergency:     "Emergency Services: Call 999",
		international: "https://www.iasp.info/resources/Crisis_Centres/",
	},
	"CA": {
		primary:       "Talk Suicide Canada: Call or text 988",
		secondary:     "Crisis Services Canada: 1-833-456-4566",
		emergency:     "Emergency Services: Call 911",
		international: "https://www.iasp.info/resources/Crisis_Centres/",
	},
	"AU": {
		primary:       "Lifeline Australia: Call 13 11 14",
		secondary:     "Beyond Blue: Call 1300 22 4636",
		emergency:     "Emergency Services: Call 000",
		international: "https://www.iasp.info/resources/Crisis_Centres/",
	},
	"NZ": {
		primary:       "Lifeline New Zealand: Call 0800 543 354",
		secondary:     "Need to Talk? Call or text 1737",
		emergency:     "Emergency Services: Call 111",
		international: "https://www.iasp.info/resources/Crisis_Centres/",
	},
	"DE": {
		primary:       "Telefonseelsorge: Call 0800 111 0 111 (free, 24/7)",
		secondary:     "Telefonseelsorge: Call 0800 111 0 222",
		emergency:     "Emergency Services: Call 112",
		international: "https://www.iasp.info/resources/Crisis_Centres/",
	},
	"FR": {
		primary:       "Numéro National de Prévention du Suicide: Call 3114",
		secondary:     "SOS Amitié: Call 09 72 39 40 50",
		emergency:     "Emergency Services: Call 15 or 112",
		international: "https://www.iasp.info/resources/Crisis_Centres/",
	},
	"IN": {
		primary:       "iCall: Call 9152987821",
		secondary:     "Vandrevala Foundation: Call 1860-2662-345 (24/7)",
		emergency:     "Emergency Services: Call 112",
		international: "https://www.iasp.info/resources/Crisis_Centres/",
	},
	"AE": {
		primary:       "Dubai Health Authority: Call 800 HOPE (4673)",
		secondary:     "Rashid Hospital Psychiatric Emergency: Call 04 219 2000",
		emergency:     "Emergency Services: Call 999",
		international: "https://www.iasp.info/resources/Crisis_Centres/",
	},
}

// warmMessage is appended to every resource block.
// It is the same in all languages — compassionate, not clinical.
const warmMessage = `
You are not alone. These feelings can change.
A trained counselor is available 24/7.`

// Resources returns formatted crisis resource text for the given country code.
// Falls back to US resources if the country is not found.
// The returned string is safe to display directly in the UI.
func Resources(country string) string {
	r, ok := resourcesByCountry[country]
	if !ok {
		r = resourcesByCountry["US"]
	}

	return fmt.Sprintf(
		"If you're in crisis, please reach out:\n\n%s\n%s\n%s\n\nFind a crisis centre near you:\n%s\n%s",
		r.primary,
		r.secondary,
		r.emergency,
		r.international,
		warmMessage,
	)
}
