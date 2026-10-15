// Runs before every build and dev server. Copies brand assets into public/ and
// publishes the self-host installer at opendesk.sh/install.sh when it exists.
import { createHash } from 'node:crypto';
import { copyFileSync, existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const site = join(dirname(fileURLToPath(import.meta.url)), '..');
const repo = join(site, '..');
const pub = join(site, 'public');
mkdirSync(pub, { recursive: true });

// Brand: favicons and app icons. Committed copies stay in place
// when the site is built on its own, outside the repo.
const brand = [
  ['brand/icons/favicon.svg', 'favicon.svg'],
  ['brand/icons/icon-16.png', 'favicon-16.png'],
  ['brand/icons/icon-32.png', 'favicon-32.png'],
  ['brand/icons/icon-48.png', 'favicon-48.png'],
  ['brand/icons/icon-192.png', 'icon-192.png'],
  ['brand/icons/icon-512.png', 'icon-512.png'],
  ['brand/icons/apple-touch-icon.png', 'apple-touch-icon.png'],
];
for (const [from, to] of brand) {
  const src = join(repo, from);
  if (existsSync(src)) copyFileSync(src, join(pub, to));
}

// Installer. The checksum is copied when the deploy step ships one, otherwise
// computed here so `sha256sum -c` always has something to check against.
const installer = join(repo, 'deploy/install/install.sh');
if (existsSync(installer)) {
  copyFileSync(installer, join(pub, 'install.sh'));
  const sum = `${installer}.sha256`;
  if (existsSync(sum)) {
    copyFileSync(sum, join(pub, 'install.sh.sha256'));
  } else {
    const hex = createHash('sha256').update(readFileSync(installer)).digest('hex');
    writeFileSync(join(pub, 'install.sh.sha256'), `${hex}  install.sh\n`);
  }
  console.log('[prebuild] published install.sh');
} else {
  console.warn('[prebuild] deploy/install/install.sh not found; opendesk.sh/install.sh is skipped in this build');
}
