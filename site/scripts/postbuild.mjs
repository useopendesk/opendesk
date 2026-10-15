// After `astro build`: hash every inline executable script in the output and write
// the CSP into dist/_headers, so the policy allows exactly what this build ships.
import { createHash } from 'node:crypto';
import { existsSync, readdirSync, readFileSync, statSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

const dist = fileURLToPath(new URL('../dist/', import.meta.url));

const files = [];
const walk = (dir) => {
  for (const name of readdirSync(dir)) {
    const p = join(dir, name);
    if (statSync(p).isDirectory()) walk(p);
    else if (p.endsWith('.html')) files.push(p);
  }
};
walk(dist);

const hashes = new Set();
const re = /<script(?![^>]*\bsrc=)([^>]*)>([\s\S]*?)<\/script>/gi;
for (const f of files) {
  const html = readFileSync(f, 'utf8');
  for (const m of html.matchAll(re)) {
    const attrs = m[1];
    if (/type=["']?application\/(ld\+)?json/i.test(attrs)) continue;
    if (!m[2].trim()) continue;
    hashes.add(`'sha256-${createHash('sha256').update(m[2], 'utf8').digest('base64')}'`);
  }
}

// PostHog, only when the build includes it.
const scriptSrc = [...hashes];
const connectSrc = [];
const key = process.env.PUBLIC_POSTHOG_KEY;
if (key) {
  const host = new URL(process.env.PUBLIC_POSTHOG_HOST || 'https://us.i.posthog.com');
  const assets = new URL(host.origin.replace('.i.posthog.com', '-assets.i.posthog.com'));
  scriptSrc.push(assets.origin);
  connectSrc.push(host.origin, assets.origin);
}

const headersPath = join(dist, '_headers');
const headers = readFileSync(headersPath, 'utf8')
  .replaceAll('__SCRIPT_SRC__', scriptSrc.join(' '))
  .replaceAll('__CONNECT_SRC__', connectSrc.join(' '))
  .replace(/ {2,}(?=;)/g, '')
  .replace(/ ;/g, ';');
writeFileSync(headersPath, headers);
console.log(`[postbuild] CSP written with ${hashes.size} inline script hash(es) across ${files.length} pages`);
if (headers.includes('__')) throw new Error('[postbuild] an unfilled placeholder is left in dist/_headers');

// Before the first desktop release, the short download links lead to /download/,
// which says the app is coming, instead of to files that do not exist yet.
const download = join(dist, 'download/index.html');
const redirectsPath = join(dist, '_redirects');
if (existsSync(download) && readFileSync(download, 'utf8').includes('data-desktop-unpublished')) {
  const redirects = readFileSync(redirectsPath, 'utf8').replace(/https:\/\/github\.com\/\S+\/releases\/download\/desktop-latest\/\S+/g, '/download/');
  writeFileSync(redirectsPath, redirects);
  console.log('[postbuild] no desktop release yet: /download/<os> leads to /download/');
}
