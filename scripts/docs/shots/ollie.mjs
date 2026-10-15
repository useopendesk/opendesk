// Shots for docs/src/content/docs/guides/ollie.mdx ("Ollie, the assistant").
import { around, block, button, labelled, settle, text } from '../helpers.mjs';

const TALL = { width: 1440, height: 1400 };

// An email ticket with a summary, notes and a full details panel.
async function openSso(page) {
  await page.getByText('SSO login loops back to sign-in page').first().click();
  await settle(page, 500);
}

// An unassigned email ticket with Ollie's suggested reply and triage on it.
async function openExport(page) {
  await page.getByText("Can't export notebook to PDF").first().click();
  await settle(page, 600);
}

const panel = (p) => p.locator('[data-ollie-panel]').first();
const olliePanel = (p) => p.locator('section[aria-label="Ollie"]').first();
const approval = (p) => around(p, 'Update #1072', 'Waiting for you');
const approve = (p) => p.locator('button').filter({ hasText: /^\s*Approve/ }).filter({ visible: true }).first();

async function ask(page, question, wait = 6500) {
  await page.locator('[data-ollie-button]').first().click();
  await settle(page, 600);
  const box = panel(page).locator('textarea').first();
  await box.fill(question);
  await box.press('Enter');
  await page.waitForTimeout(wait);
}

export const shots = [
  {
    name: 'ollie/overview',
    path: '/inbox/open',
    run: async (page) => {
      await openSso(page);
      await ask(page, 'Who should take this?');
    },
    highlight: [
      { target: (p) => p.locator('[data-ollie-button]').first(), label: 1, badge: 'bl' },
      { target: (p) => [labelled(p, 'Conversations'), labelled(p, 'New chat')], label: 2, badge: 'bl' },
      { target: (p) => [text(p, 'Read the ticket'), text(p, '5 teammates', false)], label: 3, inset: 6, badge: 'l' },
      { target: approval, label: 4, inset: 3, badge: 'l' },
      { target: (p) => panel(p).locator('textarea').first().locator('xpath=..'), label: 5, inset: 3, badge: 'l' },
    ],
  },
  {
    name: 'ollie/open',
    path: '/inbox/open',
    clip: (p) => [p.locator('[data-ollie-button]').first(), button(p, 'New ticket')],
    pad: 22,
    highlight: [{ target: (p) => p.locator('[data-ollie-button]').first(), label: 1, badge: 'bl' }],
  },
  {
    name: 'ollie/approval',
    path: '/inbox/open',
    run: async (page) => {
      await openSso(page);
      await ask(page, 'Who should take this?');
    },
    clip: (p) => [text(p, 'Who should take this?'), approval(p)],
    pad: 20,
    highlight: [
      { target: (p) => around(p, 'Assignee', 'Jonas Weber').last(), label: 1, inset: 2 },
      { target: (p) => [button(p, 'Deny'), approve(p)], label: 2, inset: 3, badge: 'bl' },
    ],
  },
  {
    name: 'ollie/ticket-panel',
    path: '/inbox/open',
    viewport: TALL,
    run: openSso,
    clip: olliePanel,
    pad: 12,
    highlight: [
      { target: (p) => around(olliePanel(p), 'All 180 Okta users', 'Updated'), label: 1, inset: { x: 9, y: 7 }, badge: 'tr' },
      { target: (p) => button(p, 'Suggest changes'), label: 2, badge: 'r' },
      { target: (p) => around(olliePanel(p), 'In your tools', 'Find related Linear issues'), label: 3, inset: { x: 9, y: 7 }, badge: 'tr' },
      { target: (p) => olliePanel(p).locator('form').last(), label: 4, inset: 4, badge: 'r' },
    ],
  },
  {
    name: 'ollie/suggest',
    path: '/inbox/open',
    viewport: TALL,
    // The first ticket where Ollie left a suggestion below the confidence threshold.
    run: async (page) => {
      const rows = page.locator('[aria-label^="Select ticket "]');
      const n = await rows.count();
      for (let i = 0; i < n; i++) {
        await rows.nth(i).locator('xpath=ancestor::*[self::a or @role="option" or self::li][1]').first().click({ force: true });
        await page.waitForTimeout(450);
        if (await olliePanel(page).getByText(/suggestions? for you/).count()) break;
      }
      await settle(page);
    },
    clip: olliePanel,
    pad: 12,
    highlight: [
      { target: (p) => [olliePanel(p).getByText('Ollie', { exact: true }).first(), olliePanel(p).getByText(/suggestions? for you/).first()], label: 1, inset: 6, badge: 'r' },
      { target: (p) => block(olliePanel(p).locator('button[aria-label^="Apply "]').first(), 240), label: 2, inset: { x: 12, y: 4 }, badge: 'r' },
    ],
  },
  {
    name: 'ollie/attribution',
    path: '/inbox/unassigned',
    viewport: TALL,
    run: openExport,
    clip: (p) => [text(p, 'suggested a reply', false), text(p, 'triaged this as', false), p.getByText('79%').first()],
    pad: 16,
    highlight: [
      { target: (p) => [text(p, 'set severity to', false), p.getByText('79%').first()], label: 1, inset: { x: 8, y: 5 } },
    ],
  },
];
