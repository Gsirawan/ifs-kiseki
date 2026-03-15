#!/usr/bin/env bash
# Record a demo GIF of IFS-Kiseki streaming chat.
# Usage: ./scripts/record-demo.sh
#
# SAFE DEMO QUESTION (copy-paste this into the chat):
#   "I've been noticing a part of me that gets really critical when I make mistakes. 
#    It tells me I should have known better. Can we explore that?"
#
# This shows IFS parts language, streaming, and the companion's warmth
# without revealing anything personal.

set -euo pipefail

OUTPUT_DIR="assets"
VIDEO_FILE="/tmp/ifs-demo-recording.mp4"
GIF_FILE="${OUTPUT_DIR}/ifs-kiseki-demo.gif"
DURATION="${1:-12}"  # seconds, default 12

# Check dependencies
for cmd in wf-recorder slurp ffmpeg; do
    if ! command -v "$cmd" &>/dev/null; then
        echo "Missing: $cmd"
        echo "Install: sudo pacman -S $cmd"
        exit 1
    fi
done

echo "╔══════════════════════════════════════════════════╗"
echo "║           IFS-Kiseki Demo Recorder               ║"
echo "╠══════════════════════════════════════════════════╣"
echo "║                                                  ║"
echo "║  SAFE QUESTION TO TYPE:                          ║"
echo "║  \"I've been noticing a part of me that gets      ║"
echo "║   really critical when I make mistakes.          ║"
echo "║   Can we explore that?\"                          ║"
echo "║                                                  ║"
echo "║  STEPS:                                          ║"
echo "║  1. Select the chat area (drag to crop)          ║"
echo "║  2. Paste the question and hit Send               ║"
echo "║  3. Let it stream for ~${DURATION} seconds               ║"
echo "║  4. Recording stops automatically                 ║"
echo "║                                                  ║"
echo "╚══════════════════════════════════════════════════╝"
echo ""
read -rp "Press Enter when ready to select area..."

# Record
echo "→ Select the area to record (drag a rectangle)..."
GEOMETRY=$(slurp)
echo "→ Recording for ${DURATION}s... (Ctrl+C to stop early)"

wf-recorder -g "$GEOMETRY" -f "$VIDEO_FILE" &
PID=$!

# Auto-stop after duration
sleep "$DURATION" 2>/dev/null && kill "$PID" 2>/dev/null || true
wait "$PID" 2>/dev/null || true

if [ ! -f "$VIDEO_FILE" ]; then
    echo "✗ Recording failed — no video file created"
    exit 1
fi

# Convert to GIF — optimized for GitHub README
echo "→ Converting to GIF..."
ffmpeg -y -i "$VIDEO_FILE" \
    -vf "fps=12,scale=800:-1:flags=lanczos,split[s0][s1];[s0]palettegen=max_colors=128:stats_mode=diff[p];[s1][p]paletteuse=dither=bayer:bayer_scale=3" \
    -loop 0 \
    "$GIF_FILE" 2>/dev/null

# Clean up
rm -f "$VIDEO_FILE"

SIZE=$(du -h "$GIF_FILE" | cut -f1)
echo ""
echo "✓ Done! ${GIF_FILE} (${SIZE})"
echo ""
echo "If file is too large (>5MB), re-run with shorter duration:"
echo "  ./scripts/record-demo.sh 8"
