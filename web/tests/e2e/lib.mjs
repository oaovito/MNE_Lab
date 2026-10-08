// Shared helpers of the browser tests. scripts/e2e.sh starts the seeded
// harness (internal/app TestBrowserHarness) and runs these scripts against it.
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { chromium } from 'playwright-core';

export const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

/** launchURL asks the harness for a fresh one-time launch URL. */
export async function launchURL(dir) {
  const want = path.join(dir, 'want-url');
  const file = path.join(dir, 'url.txt');
  fs.writeFileSync(want, '');
  for (let i = 0; i < 100; i++) {
    await sleep(100);
    if (!fs.existsSync(want) && fs.existsSync(file)) {
      const u = fs.readFileSync(file, 'utf8');
      fs.rmSync(file);
      return u;
    }
  }
  throw new Error('the harness did not answer');
}

/** launchBrowser starts Chromium (MNELAB_CHROMIUM selects a local build). */
export function launchBrowser() {
  return chromium.launch({ executablePath: process.env.MNELAB_CHROMIUM || undefined, args: ['--no-sandbox'] });
}

/** watch collects script errors, console errors and failed requests. */
export function watch(page, errors, tag) {
  page.on('pageerror', (e) => errors.push(`${tag}: script error: ${e.message}`));
  page.on('console', (m) => m.type() === 'error' && !m.text().startsWith('Failed to load resource') && errors.push(`${tag}: console: ${m.text()}`));
  page.on('response', async (r) => {
    if (r.status() >= 400) errors.push(`${tag}: ${r.status()} ${r.request().method()} ${new URL(r.url()).pathname} ${(await r.text().catch(() => '')).slice(0, 120)}`);
  });
}

/** api calls the core from inside the page (same session cookie). */
export function api(page, method, p, body) {
  return page.evaluate(
    async ([method, p, body]) => {
      const r = await fetch(p, { method, headers: { 'X-MNE-Lab': '1', 'Content-Type': 'application/json' }, body: body ? JSON.stringify(body) : undefined });
      const t = await r.text();
      try {
        return JSON.parse(t);
      } catch {
        return t;
      }
    },
    [method, p, body],
  );
}

/** Every translation key, so a key shown instead of its text is caught. */
export function translationKeys() {
  const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', '..');
  const keys = new Set();
  const dir = path.join(root, 'src', 'i18n');
  for (const f of fs.readdirSync(dir)) {
    if (!f.endsWith('.ts')) continue;
    for (const m of fs.readFileSync(path.join(dir, f), 'utf8').matchAll(/^\s*'([a-z0-9_.-]+)':/gm)) keys.add(m[1]);
  }
  const go = JSON.parse(fs.readFileSync(path.join(root, '..', 'internal', 'i18n', 'locales', 'en.json'), 'utf8'));
  for (const k of Object.keys(go)) keys.add(k);
  return keys;
}

/**
 * layoutProblems looks for what a person would see as broken: the page
 * itself scrolling or overflowing, raw translation keys, and text cut
 * without an ellipsis.
 */
export async function layoutProblems(page, keys) {
  return page.evaluate((keys) => {
    const out = [];
    // The page itself must not scroll (panels inside it may).
    const de = document.documentElement;
    scrollTo(1e6, 1e6);
    if (scrollX > 0) out.push(`page scrolls sideways (${de.scrollWidth}px in ${innerWidth}px)`);
    if (scrollY > 0) out.push(`page scrolls (${de.scrollHeight}px in ${innerHeight}px)`);
    scrollTo(0, 0);
    const known = new Set(keys);
    const text = document.body.innerText;
    for (const m of text.matchAll(/[a-z][a-z0-9_-]*(?:\.[a-z0-9_-]+)+/g)) if (known.has(m[0])) out.push(`untranslated key "${m[0]}"`);
    for (const el of document.querySelectorAll('body *')) {
      if (el.children.length || !el.textContent.trim() || !el.getClientRects().length) continue;
      const cs = getComputedStyle(el);
      if (cs.visibility === 'hidden' || cs.opacity === '0') continue;
      if (el.closest('svg, [aria-hidden="true"], .sr-only')) continue;
      const cut = (cs.overflowX === 'hidden' || cs.overflowX === 'clip') && cs.textOverflow !== 'ellipsis' && el.scrollWidth > el.clientWidth + 1;
      if (cut) out.push(`text cut: "${el.textContent.trim().slice(0, 60)}"`);
    }
    return [...new Set(out)];
  }, [...keys]);
}

export class Report {
  constructor(name) {
    this.name = name;
    this.fails = [];
    this.passed = 0;
  }
  check(what, ok, detail = '') {
    if (ok) this.passed++;
    else this.fails.push(what + (detail ? ' — ' + detail : ''));
  }
  /** finish prints the result and returns the process exit code. */
  finish(out) {
    const lines = [`${this.name}: ${this.passed} passed, ${this.fails.length} failed`, ...this.fails.map((f) => '  FAIL ' + f)];
    fs.writeFileSync(path.join(out, `${this.name}.txt`), lines.join('\n') + '\n');
    console.log(lines.join('\n'));
    return this.fails.length ? 1 : 0;
  }
}
