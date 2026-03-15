#!/usr/bin/env node
/**
 * IFS-Kiseki Demo GIF Recorder
 *
 * Fully automated — no human typing needed.
 * Launches browser, types a safe demo question, records the streaming
 * response, and converts to an optimized GIF.
 *
 * Prerequisites:
 *   - IFS-Kiseki running at http://127.0.0.1:3737
 *   - npx playwright (already installed)
 *   - ffmpeg (already installed)
 *   - API key configured and disclaimer accepted
 *
 * Usage:
 *   node scripts/record-demo.mjs
 *   node scripts/record-demo.mjs --screenshot   # also take a chat screenshot
 */

import { chromium } from 'playwright';
import { execSync } from 'child_process';
import { existsSync, unlinkSync } from 'fs';
import { resolve, dirname } from 'path';
import { fileURLToPath } from 'url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const PROJECT_ROOT = resolve(__dirname, '..');
const ASSETS_DIR = resolve(PROJECT_ROOT, 'assets');

const APP_URL = 'http://127.0.0.1:3737';
const VIDEO_DIR = '/tmp/ifs-demo-video';
const GIF_OUTPUT = resolve(ASSETS_DIR, 'ifs-kiseki-demo.gif');
const SCREENSHOT_OUTPUT = resolve(ASSETS_DIR, 'ifs-kiseki-chat.png');

// Safe, generic IFS question — shows parts language without personal info.
const DEMO_QUESTION =
  "I've been noticing a part of me that gets really critical when I make mistakes. " +
  "It tells me I should have known better. Can we explore that?";

const TAKE_SCREENSHOT = process.argv.includes('--screenshot');

async function main() {
  console.log('╔═══════════════════════════════════════════╗');
  console.log('║     IFS-Kiseki Demo Recorder              ║');
  console.log('╠═══════════════════════════════════════════╣');
  console.log('║  Fully automated — no typing needed.      ║');
  console.log('║  Safe question, no personal data.         ║');
  console.log('╚═══════════════════════════════════════════╝');
  console.log();

  // Check app is running.
  try {
    const res = await fetch(`${APP_URL}/api/health`);
    if (!res.ok) throw new Error(`Health check failed: ${res.status}`);
    console.log('✓ IFS-Kiseki is running');
  } catch {
    console.error('✗ IFS-Kiseki is not running at', APP_URL);
    console.error('  Start it first: make run');
    process.exit(1);
  }

  // Launch browser with video recording.
  console.log('→ Launching browser...');
  const browser = await chromium.launch({
    headless: false,
    executablePath: '/usr/bin/chromium',
    args: ['--disable-gpu', '--no-sandbox'],
  });

  const context = await browser.newContext({
    viewport: { width: 1280, height: 720 },
    recordVideo: { dir: VIDEO_DIR, size: { width: 1280, height: 720 } },
  });

  const page = await context.newPage();

  try {
    // Navigate to app.
    await page.goto(APP_URL, { waitUntil: 'networkidle' });
    console.log('✓ App loaded');

    // Wait for chat to be ready (connection status dot turns green).
    await page.waitForSelector('.status-connected', { timeout: 10000 });
    console.log('✓ WebSocket connected');

    // Click "+ New Session" for a clean slate.
    await page.click('#new-session-btn');
    await page.waitForTimeout(500);
    console.log('✓ New session started');

    // Type the demo question with human-like speed.
    const input = page.locator('#chat-container textarea');
    await input.click();
    await input.fill('');  // clear any existing text
    await typeHumanLike(page, input, DEMO_QUESTION);
    console.log('✓ Question typed');

    // Small pause before sending (looks natural).
    await page.waitForTimeout(400);

    // Click Send.
    await page.click('#chat-container button:has-text("Send")');
    console.log('→ Message sent, waiting for streaming response...');

    // Wait for the "thinking" indicator to appear, then for the response
    // to finish streaming. The send button gets re-enabled when done.
    await page.waitForTimeout(1000);  // let streaming start

    // Wait for streaming to finish — the send button becomes enabled again.
    await page.waitForFunction(() => {
      const btn = document.querySelector('#chat-container button');
      // Button is not disabled = streaming is done
      return btn && !btn.disabled;
    }, { timeout: 60000 });

    console.log('✓ Response complete');

    // Let the final text settle and render.
    await page.waitForTimeout(1500);

    // Take screenshot if requested.
    if (TAKE_SCREENSHOT) {
      await page.screenshot({ path: SCREENSHOT_OUTPUT, fullPage: false });
      console.log(`✓ Screenshot saved: ${SCREENSHOT_OUTPUT}`);
    }

    // Small pause at the end so the GIF doesn't cut off abruptly.
    await page.waitForTimeout(1000);

  } finally {
    // Close browser — this finalizes the video file.
    await context.close();
    await browser.close();
  }

  // Find the recorded video file.
  const videoFiles = execSync(`ls -t ${VIDEO_DIR}/*.webm 2>/dev/null || true`)
    .toString()
    .trim()
    .split('\n')
    .filter(Boolean);

  if (videoFiles.length === 0) {
    console.error('✗ No video file found in', VIDEO_DIR);
    process.exit(1);
  }

  const videoFile = videoFiles[0];
  console.log(`→ Converting ${videoFile} to GIF...`);

  // Convert to optimized GIF.
  // - fps=10: smooth enough, keeps file size down
  // - scale=800: good width for GitHub README
  // - palettegen/paletteuse: much better quality than naive conversion
  execSync(
    `ffmpeg -y -i "${videoFile}" ` +
    `-vf "fps=10,scale=800:-1:flags=lanczos,split[s0][s1];` +
    `[s0]palettegen=max_colors=128:stats_mode=diff[p];` +
    `[s1][p]paletteuse=dither=bayer:bayer_scale=3" ` +
    `-loop 0 "${GIF_OUTPUT}"`,
    { stdio: 'pipe' }
  );

  // Clean up video files.
  execSync(`rm -rf ${VIDEO_DIR}`);

  const sizeBytes = execSync(`stat -c%s "${GIF_OUTPUT}"`).toString().trim();
  const sizeMB = (parseInt(sizeBytes) / (1024 * 1024)).toFixed(1);

  console.log();
  console.log(`✓ GIF saved: ${GIF_OUTPUT} (${sizeMB} MB)`);

  if (parseFloat(sizeMB) > 5) {
    console.log('⚠ File is large (>5MB). GitHub may be slow to render it.');
    console.log('  Consider trimming the video or reducing fps.');
  }

  console.log();
  console.log('Done! Tell Lily to update the README.');
}

/**
 * Type text character by character with random delays to look human.
 */
async function typeHumanLike(page, locator, text) {
  for (const char of text) {
    await locator.press(char === ' ' ? 'Space' : char === '\n' ? 'Enter' : char, {
      delay: 0,
    });
    // Random delay between keystrokes: 30-80ms.
    await page.waitForTimeout(30 + Math.random() * 50);
  }
}

main().catch((err) => {
  console.error('✗ Error:', err.message);
  process.exit(1);
});
