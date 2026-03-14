# IFS-Kiseki — End-to-End Testing Checklist

Manual test script covering the full user journey. Execute each item in order on a clean install.

---

## Fresh Install

- [ ] Binary starts, opens browser
- [ ] Disclaimer shows on first launch
- [ ] Cannot proceed without accepting all checkboxes

## API Key Setup

- [ ] API key entry works for Claude
- [ ] API key entry works for Grok
- [ ] Invalid API key shows clear error

## Chat — Core

- [ ] Chat works with Claude (streaming)
- [ ] Chat works with Grok (streaming)
- [ ] Messages appear in real-time (streaming)

## Sessions

- [ ] Session list shows in sidebar
- [ ] Can switch between sessions
- [ ] Can start new session

## Settings

- [ ] Settings page loads and saves
- [ ] Can change provider mid-use
- [ ] Can change companion name

## Crisis Safety

- [ ] Crisis keyword "kill myself" triggers overlay
- [ ] Crisis keyword "suicide" triggers overlay
- [ ] Crisis overlay shows correct resources
- [ ] Crisis overlay cannot be dismissed for 5 seconds

## Memory

- [ ] Memory: session auto-saves on disconnect
- [ ] Memory: previous session context appears in new session
- [ ] Briefing: generates summary of past sessions

## IFS Protocol

- [ ] Companion name appears in responses
- [ ] IFS protocol: AI uses parts language naturally
- [ ] IFS protocol: AI asks about body sensations
- [ ] IFS protocol: AI checks for Self-energy
- [ ] IFS protocol: AI doesn't rush to exile work

## Build & Persistence

- [ ] Binary builds with: `go build -o ifs-kiseki .`
- [ ] Binary runs without Go installed
- [ ] Config persists across restarts
- [ ] DB persists across restarts
