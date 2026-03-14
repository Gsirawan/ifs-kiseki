package crisis

import (
	"testing"
)

// ── Test helpers ─────────────────────────────────────────────────

func newDetector() *RegexCrisisDetector {
	return NewRegexCrisisDetector("US")
}

// assertTriggers verifies that the given text IS detected as a crisis.
func assertTriggers(t *testing.T, d *RegexCrisisDetector, text string) {
	t.Helper()
	detected, category := d.Scan(text)
	if !detected {
		t.Errorf("SHOULD trigger but did NOT: %q", text)
		return
	}
	if category == "" {
		t.Errorf("triggered but category is empty for: %q", text)
	}
}

// assertNoTrigger verifies that the given text is NOT detected as a crisis.
func assertNoTrigger(t *testing.T, d *RegexCrisisDetector, text string) {
	t.Helper()
	detected, category := d.Scan(text)
	if detected {
		t.Errorf("should NOT trigger but DID (category=%q): %q", category, text)
	}
}

// ── Suicidal ideation ────────────────────────────────────────────

func TestSuicidalIdeation_KillMyself(t *testing.T) {
	d := newDetector()
	assertTriggers(t, d, "I want to kill myself")
	assertTriggers(t, d, "I'm going to kill myself tonight")
	assertTriggers(t, d, "thinking about killing myself")
}

func TestSuicidalIdeation_EndMyLife(t *testing.T) {
	d := newDetector()
	assertTriggers(t, d, "I want to end my life")
	assertTriggers(t, d, "I've decided to end my life")
}

func TestSuicidalIdeation_WantToDie(t *testing.T) {
	d := newDetector()
	assertTriggers(t, d, "I want to die")
	assertTriggers(t, d, "I wanted to die last night")
	assertTriggers(t, d, "I keep wanting to die")
}

func TestSuicidalIdeation_Suicide(t *testing.T) {
	d := newDetector()
	assertTriggers(t, d, "I'm thinking about suicide")
	assertTriggers(t, d, "I have suicidal thoughts")
	assertTriggers(t, d, "I feel suicidal")
	assertTriggers(t, d, "I've been suicidally depressed")
}

func TestSuicidalIdeation_DontWantToBeHere(t *testing.T) {
	d := newDetector()
	assertTriggers(t, d, "I don't want to be here anymore")
	assertTriggers(t, d, "I dont want to be here")
}

func TestSuicidalIdeation_NoReasonToLive(t *testing.T) {
	d := newDetector()
	assertTriggers(t, d, "I have no reason to live")
	assertTriggers(t, d, "there's no reason to live")
}

func TestSuicidalIdeation_BetterOffDead(t *testing.T) {
	d := newDetector()
	assertTriggers(t, d, "everyone would be better off dead")
	assertTriggers(t, d, "I'd be better off dead")
}

func TestSuicidalIdeation_EndItAll(t *testing.T) {
	d := newDetector()
	assertTriggers(t, d, "I just want to end it all")
	assertTriggers(t, d, "thinking about ending it all")
}

func TestSuicidalIdeation_CantGoOn(t *testing.T) {
	d := newDetector()
	assertTriggers(t, d, "I can't go on like this")
	assertTriggers(t, d, "I cannot go on")
}

func TestSuicidalIdeation_NotWorthLiving(t *testing.T) {
	d := newDetector()
	assertTriggers(t, d, "life is not worth living")
	assertTriggers(t, d, "this is not worth living")
}

func TestSuicidalIdeation_WishIWasDead(t *testing.T) {
	d := newDetector()
	assertTriggers(t, d, "I wish I was dead")
	assertTriggers(t, d, "I wish I were dead")
}

// ── Self-harm ────────────────────────────────────────────────────

func TestSelfHarm_HurtMyself(t *testing.T) {
	d := newDetector()
	assertTriggers(t, d, "I want to hurt myself")
	assertTriggers(t, d, "I've been hurting myself")
}

func TestSelfHarm_CutMyself(t *testing.T) {
	d := newDetector()
	assertTriggers(t, d, "I cut myself last night")
	assertTriggers(t, d, "I've been cutting myself")
}

func TestSelfHarm_SelfHarm(t *testing.T) {
	d := newDetector()
	assertTriggers(t, d, "I engage in self-harm")
	assertTriggers(t, d, "self harm is something I do")
	assertTriggers(t, d, "I've been self-harming")
}

func TestSelfHarm_BurningMyself(t *testing.T) {
	d := newDetector()
	assertTriggers(t, d, "I've been burning myself")
}

func TestSelfHarm_HittingMyself(t *testing.T) {
	d := newDetector()
	assertTriggers(t, d, "I keep hitting myself")
}

func TestSelfHarm_InjureMyself(t *testing.T) {
	d := newDetector()
	assertTriggers(t, d, "I want to injure myself")
	assertTriggers(t, d, "I've been injuring myself")
}

// ── Harm to others ───────────────────────────────────────────────

func TestHarmToOthers_KillSomeone(t *testing.T) {
	d := newDetector()
	assertTriggers(t, d, "I want to kill someone")
	assertTriggers(t, d, "I'm going to kill him")
	assertTriggers(t, d, "I want to kill her")
	assertTriggers(t, d, "I want to kill them")
}

func TestHarmToOthers_HurtSomeone(t *testing.T) {
	d := newDetector()
	assertTriggers(t, d, "I want to hurt someone")
	assertTriggers(t, d, "I'm going to hurt him")
}

func TestHarmToOthers_HarmSomeone(t *testing.T) {
	d := newDetector()
	assertTriggers(t, d, "I want to harm someone")
	assertTriggers(t, d, "I want to harm her")
}

func TestHarmToOthers_WantToHurt(t *testing.T) {
	d := newDetector()
	assertTriggers(t, d, "I want to hurt someone")
	assertTriggers(t, d, "I really want to hurt him")
	assertTriggers(t, d, "I want to hurt her so badly")
}

func TestHarmToOthers_GoingToHurt(t *testing.T) {
	d := newDetector()
	assertTriggers(t, d, "I'm going to hurt someone")
	assertTriggers(t, d, "I'm going to hurt him")
}

// ── Acute crisis ─────────────────────────────────────────────────

func TestAcuteCrisis_Overdose(t *testing.T) {
	d := newDetector()
	assertTriggers(t, d, "I took an overdose")
	assertTriggers(t, d, "I'm overdosing")
	assertTriggers(t, d, "I overdosed last week")
}

func TestAcuteCrisis_TookPills(t *testing.T) {
	d := newDetector()
	assertTriggers(t, d, "I took too many pills")
	assertTriggers(t, d, "I took pills")
	assertTriggers(t, d, "I'm taking pills to end it")
	assertTriggers(t, d, "I swallowed pills")
}

// ── Case insensitivity ───────────────────────────────────────────

func TestCaseInsensitivity(t *testing.T) {
	d := newDetector()
	assertTriggers(t, d, "I WANT TO KILL MYSELF")
	assertTriggers(t, d, "I Want To Die")
	assertTriggers(t, d, "SUICIDE")
	assertTriggers(t, d, "Self-Harm")
	assertTriggers(t, d, "OVERDOSE")
}

// ── Multi-pattern messages ────────────────────────────────────────

func TestMultiPattern(t *testing.T) {
	d := newDetector()
	// Message containing multiple crisis indicators — should still trigger.
	assertTriggers(t, d, "I feel suicidal and I've been cutting myself")
	assertTriggers(t, d, "I want to die and I took too many pills")
}

// ── False positives — MUST NOT trigger ───────────────────────────

func TestFalsePositive_KilledItAtWork(t *testing.T) {
	d := newDetector()
	// "killed it" — idiom for success, no person pronoun after "kill"
	assertNoTrigger(t, d, "I killed it at work today")
	assertNoTrigger(t, d, "We absolutely killed it at the presentation")
	assertNoTrigger(t, d, "She killed it on stage last night")
}

func TestFalsePositive_DyingOfLaughter(t *testing.T) {
	d := newDetector()
	assertNoTrigger(t, d, "I'm dying of laughter")
	assertNoTrigger(t, d, "I was dying laughing at that meme")
}

func TestFalsePositive_DieToDie(t *testing.T) {
	d := newDetector()
	// "to die for" — food/fashion idiom
	assertNoTrigger(t, d, "this pizza is to die for")
	assertNoTrigger(t, d, "that dress is to die for")
}

func TestFalsePositive_HurtSomeoneFeelings(t *testing.T) {
	// "hurt someone's feelings" — our pattern includes (?:'s)? so this WILL trigger.
	// This is an intentional design decision: in a therapy context, "I want to hurt
	// someone's feelings" is ambiguous enough that showing resources is acceptable.
	// The spec says: "Better to show resources unnecessarily than miss a real crisis."
	// We document this as a known acceptable false positive — do not test as no-trigger.
	d := newDetector()
	_ = d
}

func TestFalsePositive_KillItGame(t *testing.T) {
	d := newDetector()
	// "kill it" — gaming/music idiom. "it" is NOT in our person pronoun list,
	// so these should NOT trigger.
	assertNoTrigger(t, d, "I'm going to kill it in this game")
	assertNoTrigger(t, d, "let's kill it tonight on stage")
	assertNoTrigger(t, d, "we're going to kill it at the concert")
}

func TestFalsePositive_EndOfDay(t *testing.T) {
	d := newDetector()
	assertNoTrigger(t, d, "at the end of the day")
	assertNoTrigger(t, d, "by the end of my shift")
}

func TestFalsePositive_CantGoOnVacation(t *testing.T) {
	d := newDetector()
	// "can't go on" in a non-crisis context — this WILL trigger because
	// the phrase is identical. This is intentional: better safe than sorry.
	// The plan says: "no false negatives preferred over no false positives."
	// So we do NOT test this as a false positive — it's an acceptable trigger.
	_ = d
}

func TestFalsePositive_PillsForHeadache(t *testing.T) {
	d := newDetector()
	// "took pills" for a headache — this WILL trigger (acceptable per spec).
	// The spec says: "Better to show resources unnecessarily than miss a real crisis."
	// We document this as a known acceptable false positive.
	_ = d
}

func TestFalsePositive_HurtMyBack(t *testing.T) {
	d := newDetector()
	// "hurt my back/knee" — our harm-to-others patterns require a person pronoun
	// (someone/him/her/them/you), not "my [body part]". These should NOT trigger.
	// "hurt myself" is in self-harm patterns but requires the word "myself".
	assertNoTrigger(t, d, "I hurt my back at the gym")
	assertNoTrigger(t, d, "I hurt my knee running")
	assertNoTrigger(t, d, "I hurt my shoulder lifting")
}

func TestFalsePositive_CutMyFinger(t *testing.T) {
	d := newDetector()
	// "cut my finger" — pattern requires "cut myself", not "cut my finger"
	assertNoTrigger(t, d, "I cut my finger while cooking")
	assertNoTrigger(t, d, "I cut my hand on the glass")
}

func TestFalsePositive_BurnedMyself_Cooking(t *testing.T) {
	d := newDetector()
	// "burned myself cooking" — this WILL trigger (burning myself pattern).
	// Acceptable per spec — better safe than sorry in a therapy context.
	_ = d
}

func TestFalsePositive_EmptyString(t *testing.T) {
	d := newDetector()
	assertNoTrigger(t, d, "")
}

func TestFalsePositive_NormalTherapyTalk(t *testing.T) {
	d := newDetector()
	assertNoTrigger(t, d, "I feel anxious about my job interview")
	assertNoTrigger(t, d, "My inner critic is very loud today")
	assertNoTrigger(t, d, "I'm working on my perfectionism")
	assertNoTrigger(t, d, "I feel disconnected from my body")
	assertNoTrigger(t, d, "There's a part of me that feels scared")
}

// ── Scan return values ────────────────────────────────────────────

func TestScanReturnsCategoryName(t *testing.T) {
	d := newDetector()

	cases := []struct {
		text             string
		expectedCategory string
	}{
		{"I want to kill myself", "suicidal ideation"},
		{"I've been cutting myself", "self-harm"},
		{"I want to kill someone", "harm to others"},
		{"I took an overdose", "acute crisis"},
	}

	for _, tc := range cases {
		detected, category := d.Scan(tc.text)
		if !detected {
			t.Errorf("expected trigger for %q, got none", tc.text)
			continue
		}
		if category != tc.expectedCategory {
			t.Errorf("text=%q: expected category %q, got %q", tc.text, tc.expectedCategory, category)
		}
	}
}

func TestScanNoTriggerReturnsEmptyCategory(t *testing.T) {
	d := newDetector()
	detected, category := d.Scan("I feel a bit sad today")
	if detected {
		t.Errorf("expected no trigger, got category=%q", category)
	}
	if category != "" {
		t.Errorf("expected empty category, got %q", category)
	}
}

// ── Resources ────────────────────────────────────────────────────

func TestResourcesNotEmpty(t *testing.T) {
	d := newDetector()
	r := d.Resources()
	if r == "" {
		t.Error("Resources() returned empty string")
	}
}

func TestResourcesContains988(t *testing.T) {
	d := newDetector()
	r := d.Resources()
	if len(r) == 0 {
		t.Fatal("Resources() is empty")
	}
	// US resources must include 988.
	found := false
	for i := 0; i < len(r)-2; i++ {
		if r[i] == '9' && r[i+1] == '8' && r[i+2] == '8' {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("US resources should contain '988', got:\n%s", r)
	}
}

func TestResourcesContainsWarmMessage(t *testing.T) {
	d := newDetector()
	r := d.Resources()
	if len(r) == 0 {
		t.Fatal("Resources() is empty")
	}
	// Must contain the warm message.
	needle := "You are not alone"
	found := false
	for i := 0; i <= len(r)-len(needle); i++ {
		if r[i:i+len(needle)] == needle {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Resources() should contain %q, got:\n%s", needle, r)
	}
}

func TestResourcesFallbackToUS(t *testing.T) {
	d := NewRegexCrisisDetector("XX") // unknown country
	r := d.Resources()
	if r == "" {
		t.Error("Resources() returned empty string for unknown country")
	}
	// Should fall back to US — contains 988.
	found := false
	for i := 0; i < len(r)-2; i++ {
		if r[i] == '9' && r[i+1] == '8' && r[i+2] == '8' {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("fallback to US should contain '988', got:\n%s", r)
	}
}

func TestResourcesKnownCountries(t *testing.T) {
	countries := []string{"US", "GB", "CA", "AU", "NZ", "DE", "FR", "IN", "AE"}
	for _, country := range countries {
		d := NewRegexCrisisDetector(country)
		r := d.Resources()
		if r == "" {
			t.Errorf("Resources() empty for country %q", country)
		}
	}
}
