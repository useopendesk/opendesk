// Shots for docs/src/content/docs/getting-started/quickstart-cloud.mdx.
import { around, button, settle } from '../helpers.mjs';

// The mock app runs on the shot server's origin; a cloud reader sees app.opendesk.sh in
// the same places (the help center address, the widget snippet).
async function cloudHost(page) {
  await page.evaluate(() => {
    const walk = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
    for (let n = walk.nextNode(); n; n = walk.nextNode()) {
      const t = n.textContent;
      if (!/127\.0\.0\.1:\d+|localhost:\d+/.test(t)) continue;
      n.textContent = t.replace(/https?:\/\/(127\.0\.0\.1|localhost):\d+/g, 'https://app.opendesk.sh').replace(/(127\.0\.0\.1|localhost):\d+/g, 'app.opendesk.sh');
    }
  });
}

const steps = (page) => page.locator('ol[aria-label="Setup progress"]');

export const shots = [
  {
    name: 'start/sign-up',
    path: '/register?mock=anon',
    run: async (page) => {
      await page.getByLabel('Name', { exact: true }).fill('Maya Chen');
      await page.getByLabel('Work email').fill('maya@margin.app');
      await page.getByLabel('Password', { exact: true }).fill('quiet-harbour-lantern-42');
      await page.getByLabel('Password', { exact: true }).blur();
    },
    clip: (p) => [p.getByRole('heading', { name: 'Create your account' }), button(p, 'Sign up with Google'), button(p, 'Create account'), p.getByText('Already have an account?')],
    pad: 28,
    highlight: [
      { target: (p) => [button(p, 'Sign up with Google'), button(p, 'Sign up with SSO')], label: 1, badge: 'l', inset: 5 },
      { target: (p) => [p.getByText('Name', { exact: true }), p.getByLabel('Password', { exact: true })], label: 2, badge: 'l', inset: { x: 10, y: 8 } },
      { target: (p) => button(p, 'Create account'), label: 3, badge: 'l' },
    ],
  },
  {
    name: 'start/workspace',
    path: '/onboarding',
    run: async (page) => {
      await page.getByLabel('Workspace name').fill('Margin');
      await page.getByLabel('Workspace name').blur();
      await cloudHost(page);
    },
    clip: (p) => [steps(p), button(p, 'Continue')],
    pad: 28,
    highlight: [
      { target: (p) => p.getByLabel('Workspace name'), label: 1, badge: 'l' },
      { target: (p) => p.getByText('Your help center', { exact: true }).locator('xpath=..'), label: 2, badge: 'l' },
      { target: (p) => button(p, 'Continue'), label: 3, badge: 'l' },
    ],
  },
  {
    name: 'start/mailbox',
    path: '/onboarding/mailbox',
    // Email first, as on opendesk Cloud: the mock reads localStorage["od.mock.mailboxOAuth"]
    // for the provider sign-in the instance offers, and Cloud offers Microsoft only.
    viewport: { width: 1440, height: 1300 },
    run: async (page) => {
      await page.evaluate(() => localStorage.setItem('od.mock.mailboxOAuth', 'microsoft'));
      await page.reload();
      await settle(page);
      await page.getByLabel('Name', { exact: true }).fill('Care');
      await page.getByLabel('Email address', { exact: true }).fill('care@margin.app');
      await page.getByLabel('Email address', { exact: true }).blur();
      await page.getByRole('radiogroup').waitFor();
      await settle(page, 500);
    },
    clip: (p) => [steps(p), button(p, 'Continue'), p.getByRole('link', { name: 'Skip for now' })],
    pad: 28,
    highlight: [
      { target: (p) => [p.getByLabel('Name', { exact: true }), p.getByLabel('Email address', { exact: true })], label: 1, badge: 'l', inset: { x: 8, y: 3 } },
      { target: (p) => [p.getByText("Detected from the domain's MX records", { exact: true }).locator('xpath=ancestor::div[contains(@class, "rounded-lg")][1]'), p.getByRole('radiogroup')], label: 2, badge: 'l', inset: { x: 6, y: 2 } },
      { target: (p) => button(p, 'Continue'), label: 3, badge: 'l' },
      { target: (p) => p.getByRole('link', { name: 'Skip for now' }), label: 4, badge: 'r' },
    ],
  },
  {
    name: 'start/widget',
    path: '/settings/widget',
    run: async (page) => {
      await page.getByText('Install', { exact: true }).first().scrollIntoViewIfNeeded();
      await page.mouse.move(700, 500);
      await page.mouse.wheel(0, 260);
      await settle(page);
      await cloudHost(page);
    },
    // The settings menu and the Install section; the preview column is left out so the
    // snippet stays readable at page width.
    clip: (p) => [p.getByRole('link', { name: 'Profile', exact: true }), p.getByRole('link', { name: 'Brands and widgets', exact: true }), around(p, 'Install', 'Widget key')],
    pad: 8,
    highlight: [
      { target: (p) => p.getByRole('link', { name: 'Brands and widgets', exact: true }), label: 1, badge: 'r', inset: 2 },
      { target: (p) => around(p, 'Install', 'Widget key'), label: 2, badge: 'tl', inset: { x: 12, y: 8 } },
    ],
  },
  {
    name: 'start/invite',
    path: '/settings/members',
    run: async (page) => {
      await button(page, 'Invite people').click();
      await settle(page);
      await page.keyboard.type('lea@margin.app sam@margin.app ');
      await settle(page);
    },
    keepFocus: true,
    clip: (p) => p.getByRole('dialog'),
    pad: 12,
    highlight: [
      { target: (p) => p.getByRole('dialog').getByText('Email addresses', { exact: true }).locator('xpath=..'), label: 1, badge: 'l', inset: { x: 10, y: 6 } },
      { target: (p) => [p.getByRole('radio', { name: /Admin/ }), p.getByRole('radio', { name: /Viewer/ })], label: 2, badge: 'l', inset: 4 },
      { target: (p) => button(p, 'Send invitations'), label: 3, badge: 'l' },
    ],
  },
];
