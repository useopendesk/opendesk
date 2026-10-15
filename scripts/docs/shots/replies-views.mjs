// Shots for docs/src/content/docs/guides/saved-replies-and-views.mdx.
import { around, button, labelled, link, settle } from '../helpers.mjs';

// The saved replies card: the list on the left, the editor on the right.
const card = (p) => p.locator('input[placeholder="Find a reply"]').locator('xpath=ancestor::div[contains(@class, "rounded")][last()]');
const list = (p) => p.locator('input[placeholder="Find a reply"]').locator('xpath=ancestor::div[contains(@class, "border-r")][1]');
const offeredOn = (p) => p.getByText('Offered on', { exact: true }).locator('xpath=..');
const editorBox = (p) => p.locator('[contenteditable="true"]').first().locator('xpath=ancestor::div[contains(@class, "border")][1]');
const composer = (p) => around(p, 'Replying as', 'Send');

async function openTicket(page) {
  await page.getByText('SSO login loops back to sign-in page').first().click();
  await settle(page, 800);
}

// Opens a drawer or dialog, clicking again if the first click came before the page was ready.
export async function openDrawer(page, buttonName, placeholder) {
  for (let i = 0; i < 3; i++) {
    await button(page, buttonName).click();
    try {
      await page.getByPlaceholder(placeholder).first().waitFor({ timeout: 4000 });
      break;
    } catch {}
  }
  await settle(page);
}

// The views list: one card with a row per view.
const viewRow = (p, name) => p.locator('main li, main [data-sortable-item], main div').filter({ has: p.getByText(name, { exact: true }) }).filter({ has: p.locator('button[aria-label^="Actions for"]') }).last();

export const shots = [
  {
    name: 'replies-views/replies',
    path: '/settings/saved-replies',
    clip: card,
    pad: 28,
    highlight: [
      { target: list, label: 1, inset: -2 },
      { target: (p) => [p.getByText('Title', { exact: true }), p.locator('input[value="repro"]')], label: 2, inset: 6, badge: 'tl' },
      { target: offeredOn, label: 3, inset: 6, badge: 'tr' },
      { target: editorBox, label: 4, inset: 3, badge: 'bl' },
    ],
  },
  {
    name: 'replies-views/offered-on',
    path: '/settings/saved-replies',
    run: async (page) => {
      await offeredOn(page).getByText('Everyone', { exact: true }).click();
      await settle(page);
    },
    keepFocus: true,
    clip: (p) => [p.getByText('Title', { exact: true }), offeredOn(p), p.locator('[data-radix-popper-content-wrapper]').last()],
    pad: 24,
    highlight: [
      { target: offeredOn, label: 1, inset: 6 },
      { target: (p) => p.locator('[data-radix-popper-content-wrapper] > *').last(), label: 2, inset: 3 },
    ],
  },
  {
    name: 'replies-views/composer-slash',
    path: '/inbox/open',
    run: async (page) => {
      await openTicket(page);
      await page.locator('[contenteditable="true"]').first().click();
      await page.keyboard.type('/');
      await settle(page, 500);
    },
    keepFocus: true,
    clip: (p) => [composer(p), around(p, 'Saved replies', 'Ask for steps to reproduce')],
    pad: 20,
    highlight: [{ target: (p) => around(p, 'Saved replies', 'Ask for steps to reproduce', 'Closing, no reply'), label: 1, inset: 2 }],
  },
  {
    name: 'replies-views/composer-button',
    path: '/inbox/open',
    run: async (page) => {
      await openTicket(page);
      await labelled(page, 'Saved replies').click();
      await settle(page, 500);
    },
    keepFocus: true,
    clip: (p) => [composer(p), p.locator('[data-radix-popper-content-wrapper]').last()],
    pad: 20,
    highlight: [
      { target: (p) => labelled(p, 'Saved replies'), label: 1 },
      { target: (p) => p.locator('[data-radix-popper-content-wrapper] > *').last(), label: 2, inset: 3, badge: 'tr' },
    ],
  },
  {
    name: 'replies-views/views',
    path: '/settings/views',
    clip: (p) => [p.locator('h1').last(), button(p, 'New view'), viewRow(p, 'Legal requests due')],
    pad: 28,
    highlight: [
      { target: (p) => button(p, 'New view'), label: 1, badge: 'l' },
      { target: (p) => p.getByText('Shared', { exact: true }).first(), label: 2, badge: 'tr', inset: { x: 6, y: 4 } },
      { target: (p) => p.getByText('Only you', { exact: true }).first(), label: 3, badge: 'tr', inset: { x: 6, y: 4 } },
      { target: (p) => viewRow(p, 'Legal requests due').getByText('Legal', { exact: true }).locator('xpath=..'), label: 4, inset: 3 },
      { target: (p) => p.locator('main button[role="switch"]').first(), label: 5, badge: 'l' },
    ],
  },
  {
    name: 'replies-views/view-editor',
    path: '/settings/views',
    run: async (page) => {
      await openDrawer(page, 'New view', 'Billing queue');
      await page.getByPlaceholder('Billing queue').fill('Waiting on billing');
      await page.locator('[role="dialog"] button, [role="dialog"] [role="radio"]').filter({ hasText: /^\s*A team\s*$/ }).first().click();
      await settle(page);
    },
    clip: (p) => p.getByRole('dialog'),
    pad: 0,
    highlight: [
      { target: (p) => around(p, 'Who sees this view', 'Everyone on the team sees it'), label: 1, inset: 3 },
      { target: (p) => around(p, 'Status', 'Priority', 'Severity', 'Keyword').last(), label: 2, inset: -3 },
      { target: (p) => button(p, 'Create view'), label: 3, badge: 'l' },
    ],
  },
  {
    name: 'replies-views/sidebar',
    path: '/inbox/open',
    dim: true,
    clip: () => [{ x: 0, y: 300, width: 1, height: 1 }, { x: 240, y: 615, width: 1, height: 1 }],
    pad: 0,
    highlight: [{ target: (p) => [link(p, 'Urgent'), link(p, 'Legal requests due')], label: 1, inset: 3, badge: 'tr' }],
  },
];
