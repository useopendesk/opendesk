// Shots for docs/src/content/docs/guides/desktop-app.mdx. The desktop app shows the
// instance's own web app, so these load the mock app with a stand-in desktop bridge
// (window.opendeskDesktop, src/lib/desktop.ts) and frame it the way the macOS window
// draws it (desktop/src/main/devcapture.ts): the backdrop behind the translucent
// chrome, and the traffic lights. Electron itself never runs.
import { settle } from '../helpers.mjs';

const STEADY = `*,*::before,*::after{animation-duration:0s!important;transition-duration:0s!important;caret-color:transparent!important}*{scrollbar-width:none!important}`;
const WIN = { x: 48, y: 40, width: 1344, height: 820 };

/** Installs a desktop bridge in every frame of the page, with this update state. */
async function bridge(page, update) {
  await page.addInitScript((update) => {
    const listeners = new Set();
    window.opendeskDesktop = {
      version: '1.2.4',
      platform: 'darwin',
      notify() {},
      setBadge() {},
      oauth: async () => ({ error: 'unavailable' }),
      openExternal() {},
      switchServer() {},
      onNavigate: () => () => {},
      update: {
        state: () => ({ ...update }),
        onChange: (cb) => (listeners.add(cb), () => listeners.delete(cb)),
        restart() {},
        check() {},
        channel: () => 'stable',
        setChannel() {},
      },
    };
  }, update);
}

const app = (p) => p.frameLocator('iframe#app');

/** Replaces the page with a desktop backdrop and a window that holds the app at `path`. */
async function windowed(page, { theme }, update, path = '/inbox/open') {
  await bridge(page, update);
  const dark = theme === 'dark';
  const bg = dark
    ? 'radial-gradient(120% 90% at 10% 0%, #3a3a46 0%, #26262c 45%, #1e1e22 100%)'
    : 'radial-gradient(120% 90% at 10% 0%, #e6e8f0 0%, #ececee 45%, #e9e9ea 100%)';
  const desk = dark ? 'linear-gradient(160deg, #23232a, #15151a)' : 'linear-gradient(160deg, #dfe3ec, #c9cfdc)';
  const lights = ['#ff5f57', '#febc2e', '#28c840']
    .map((c, i) => `<i style="position:absolute;top:18px;left:${18 + i * 20}px;width:12px;height:12px;border-radius:50%;background:${c};box-shadow:inset 0 0 0 0.5px rgb(0 0 0/.15)"></i>`)
    .join('');
  const src = new URL(`${path}${path.includes('?') ? '&' : '?'}mock_quiet=1`, page.url()).toString();
  await page.setContent(
    `<html><body style="margin:0;height:100vh;background:${desk};overflow:hidden">
      <div id="win" style="position:absolute;left:${WIN.x}px;top:${WIN.y}px;width:${WIN.width}px;height:${WIN.height}px;border-radius:12px;overflow:hidden;background:${bg};box-shadow:0 0 0 0.5px rgb(0 0 0/${dark ? '.6' : '.18'}),0 22px 70px rgb(0 0 0/${dark ? '.55' : '.25'})">
        <iframe id="app" src="${src}" style="position:absolute;inset:0;width:100%;height:100%;border:0;background:transparent" allowtransparency="true"></iframe>
        ${lights}
      </div>
    </body></html>`,
  );
  const frame = page.frames().find((f) => f !== page.mainFrame());
  await frame.waitForLoadState('networkidle');
  await frame.addStyleTag({ content: STEADY });
  await frame.evaluate(() => document.fonts.ready);
  await settle(page, 600);
  return frame;
}

const READY = { status: 'ready', version: '1.3.0', notes_url: 'https://github.com/useopendesk/opendesk/releases' };
const banner = (p) => app(p).locator('[role="status"]').filter({ hasText: 'opendesk 1.' }).first();
// The top of the window down to the bottom of the update strip, with some desk around it.
const topTo = async (strip) => {
  const b = await strip.boundingBox();
  return { x: WIN.x - 16, y: WIN.y - 16, width: WIN.width + 32, height: b.y + b.height - WIN.y + 16 };
};
const top = (p) => topTo(banner(p));
const notes = (p) => app(p).locator('a, button').filter({ hasText: "What's new" }).first();

export const shots = [
  {
    name: 'desktop/window',
    path: '/inbox/open',
    run: (page, ctx) => windowed(page, ctx, READY),
    highlight: [
      { target: banner, label: 1, inset: 3, badge: 'bl' },
      { target: (p) => app(p).locator('button[aria-label*="otifications"]').first(), label: 2, inset: 3, badge: 'bl' },
    ],
  },
  {
    name: 'desktop/update-ready',
    path: '/inbox/open',
    run: (page, ctx) => windowed(page, ctx, READY),
    clip: top,
    pad: 0,
    highlight: [
      { target: (p) => app(p).getByRole('button', { name: 'Restart' }), label: 1, inset: 3, badge: 'bl' },
      { target: notes, label: 2, inset: 3, badge: 'bl' },
    ],
  },
  {
    name: 'desktop/update-downloading',
    path: '/inbox/open',
    run: (page, ctx) => windowed(page, ctx, { status: 'downloading', version: '1.3.0', percent: 62 }),
    clip: (p) => topTo(app(p).locator('[role="status"]').filter({ hasText: 'Downloading' }).first()),
    pad: 0,
  },
  {
    name: 'desktop/switch-server',
    path: '/inbox/open',
    run: async (page, ctx) => {
      await windowed(page, ctx, { status: 'idle' });
      await app(page).getByText('maya@margin.app', { exact: true }).last().click();
      await settle(page, 500);
    },
    // Focus stays in the window's frame, or the menu closes.
    keepFocus: true,
    clip: (p) => [app(p).locator('[role="menu"]').last(), app(p).getByText('maya@margin.app', { exact: true }).last()],
    pad: 22,
    highlight: [{ target: (p) => app(p).locator('[role="menuitem"]').filter({ hasText: 'Switch server' }), label: 1, inset: 3, badge: 'r' }],
  },
];
