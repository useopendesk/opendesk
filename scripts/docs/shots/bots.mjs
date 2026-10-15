// Shots for docs/src/content/docs/guides/bots.mdx: the bots list, the builder's Look,
// Personality and Test tabs, picking a bot for a widget and a mailbox, and a chat
// answered by a custom bot.
import { around, button, labelled, settle, text } from '../helpers.mjs';
import { framed, row, title } from './ai.mjs';

const TALL = { width: 1440, height: 1400 };
const main = (p) => p.locator('main');
const botRow = (p, name) => main(p).locator('a[href*="/settings/ai/bots/"]').filter({ hasText: name }).first();
const tab = (p, name) => p.getByRole('tab', { name, exact: true });
const radio = (p, group, name) => p.locator(`[role="radiogroup"][aria-label="${group}"] [role="radio"][aria-label="${name}"]`).first();
const preview = (p) => framed(p.getByText('How it greets', { exact: true }).first(), 280);

async function openBot(page, name, tabName) {
  await botRow(page, name).click();
  await settle(page, 500);
  if (tabName) {
    await tab(page, tabName).click();
    await settle(page, 400);
  }
}

export const shots = [
  {
    name: 'bots/overview',
    path: '/settings/ai/bots',
    clip: (p) => [title(p, 'Bots'), button(p, 'New bot'), text(p, 'Bots are what customers see', false)],
    pad: 22,
    highlight: [
      { target: (p) => button(p, 'New bot'), label: 1, badge: 'l' },
      { target: (p) => botRow(p, 'Ollie').getByText('Default', { exact: true }), label: 2, badge: 'tr' },
      { target: (p) => botRow(p, 'Pip').getByText('Answers on Docs', { exact: true }), label: 3, inset: { x: 6, y: 3 }, badge: 'r' },
      { target: (p) => labelled(p, 'Actions for Margin Concierge'), label: 4, badge: 'l' },
    ],
  },
  {
    name: 'bots/open-settings',
    path: '/settings/ai/bots',
    dim: true,
    clip: () => [{ x: 0, y: 0, width: 1, height: 1 }, { x: 860, y: 999, width: 1, height: 1 }],
    viewport: { width: 1440, height: 1000 },
    pad: 0,
    highlight: [
      { target: (p) => p.getByRole('link', { name: 'Settings', exact: true }).last(), label: 1, badge: 'tr' },
      { target: (p) => p.getByRole('link', { name: 'Bots', exact: true }).last(), label: 2, badge: 'r' },
    ],
  },
  {
    name: 'bots/look',
    path: '/settings/ai/bots',
    viewport: TALL,
    run: (page) => openBot(page, 'Pip'),
    clip: (p) => [tab(p, 'Look'), framed(p.getByText('Style', { exact: true }).first(), 400), preview(p)],
    pad: 22,
    highlight: [
      { target: (p) => p.getByRole('radio', { name: 'Built', exact: true }).locator('xpath=..'), label: 1, badge: 'l' },
      { target: (p) => p.locator('[role="radiogroup"][aria-label="Eyes"]'), label: 2, inset: 6, badge: 'l' },
      { target: (p) => p.locator('[role="radiogroup"][aria-label="Colour"]'), label: 3, inset: 6, badge: 'l' },
      { target: (p) => button(p, 'Shuffle'), label: 4, badge: 'l' },
      { target: (p) => p.getByRole('radio', { name: 'Thinking', exact: true }).locator('xpath=..'), label: 5, badge: 'r' },
    ],
  },
  {
    name: 'bots/shuffle',
    path: '/settings/ai/bots',
    run: async (page) => {
      await openBot(page, 'Margin Concierge');
      for (let i = 0; i < 4; i++) await page.keyboard.press('r');
      await settle(page, 800);
    },
    clip: preview,
    pad: 20,
    highlight: [
      { target: (p) => button(p, 'Shuffle'), label: 1, badge: 'l' },
      { target: (p) => labelled(p, 'Undo look change'), label: 2, badge: 'tr' },
    ],
  },
  {
    name: 'bots/emoji',
    path: '/settings/ai/bots',
    viewport: TALL,
    run: async (page) => {
      await openBot(page, 'Pip');
      await page.getByRole('radio', { name: 'Emoji', exact: true }).click();
      await radio(page, 'Pick an emoji', '🦉').click().catch(() => {});
      await settle(page, 600);
    },
    clip: (p) => [p.getByRole('radio', { name: 'Built', exact: true }), p.locator('[aria-label="Pick an emoji"]'), p.locator('[aria-label="Or type your own"]'), preview(p)],
    highlight: [
      { target: (p) => p.locator('[aria-label="Pick an emoji"]'), label: 1, inset: 6, badge: 'l' },
      { target: (p) => p.locator('[aria-label="Or type your own"]'), label: 2, inset: 4, badge: 'r' },
    ],
    pad: 22,
  },
  {
    name: 'bots/personality',
    path: '/settings/ai/bots',
    viewport: { width: 1440, height: 2200 },
    run: (page) => openBot(page, 'Margin Concierge', 'Personality'),
    clip: (p) => [tab(p, 'Personality'), p.locator('#bot-signoff'), preview(p)],
    pad: 22,
    highlight: [
      { target: (p) => p.locator('[role="radiogroup"][aria-label="Tone"]'), label: 1, inset: 6, badge: 'l' },
      { target: (p) => [row(p, 'Reply length'), row(p, 'Emoji')], label: 2, inset: 4, badge: 'l' },
      { target: (p) => p.locator('#bot-greeting'), label: 3, inset: 4, badge: 'l' },
      { target: (p) => p.locator('#bot-signoff'), label: 4, inset: 4, badge: 'l' },
    ],
  },
  {
    name: 'bots/instructions',
    path: '/settings/ai/bots',
    viewport: { width: 1440, height: 2200 },
    run: (page) => openBot(page, 'Pip', 'Personality'),
    clip: (p) => [p.getByText('Instructions', { exact: true }).first(), p.getByText('Topics it declines', { exact: true }).first(), text(p, 'unreleased features')],
    pad: 22,
    highlight: [
      { target: (p) => p.locator('#bot-instructions'), label: 1, inset: 4, badge: 'l' },
      { target: (p) => text(p, 'unreleased features').locator('xpath=..'), label: 2, inset: 4, badge: 'r' },
    ],
  },
  {
    name: 'bots/test',
    path: '/settings/ai/bots',
    viewport: { width: 1440, height: 1100 },
    run: async (page) => {
      await openBot(page, 'Pip', 'Test');
      await settle(page, 1500);
      const box = page.getByPlaceholder('Write as a visitor');
      await box.fill('How do I set up SSO with Okta?');
      await box.press('Enter');
      await page.waitForTimeout(6000);
    },
    clip: (p) => [text(p, 'In the widget'), text(p, 'Test chats are not saved as tickets', false)],
    pad: 22,
    highlight: [
      { target: (p) => around(p, 'In the widget', 'Light', 'Dark').locator('xpath=following-sibling::*[1]'), label: 1, inset: 3 },
      { target: (p) => p.getByPlaceholder('Write as a visitor'), label: 2, inset: 4, badge: 'l' },
    ],
  },
  {
    name: 'bots/widget-picker',
    path: '/settings/widget',
    viewport: { width: 1440, height: 1000 },
    run: async (page) => {
      await settle(page, 1200);
      await page.locator('#widget-bot').click();
      await settle(page, 400);
    },
    clip: (p) => [p.getByText('Its name and where its conversations go.', { exact: true }).first(), p.locator('#widget-bot'), p.locator('[role="listbox"]')],
    pad: 22,
    keepFocus: true,
    highlight: [{ target: (p) => p.locator('#widget-bot'), label: 1, inset: 4, badge: 'l' }],
  },
  {
    name: 'bots/mailbox-picker',
    path: '/settings/mailboxes',
    viewport: { width: 1440, height: 2600 },
    run: async (page) => {
      await page.getByText('hello@pressroom.app', { exact: true }).first().click();
      await settle(page, 700);
      // The bot picker shows once the agent answers this address.
      const toggle = page.getByRole('switch', { name: /answers emails to this mailbox/ });
      if ((await toggle.getAttribute('aria-checked')) !== 'true') await toggle.click();
      await settle(page, 300);
      await page.locator('#mailbox-bot').scrollIntoViewIfNeeded();
      await settle(page, 300);
    },
    clip: (p) => [p.locator('h2, h3').filter({ hasText: /^\s*(Ollie|Pip|Margin Concierge)\s*$/ }).last(), framed(p.getByText('Draft for review', { exact: true }).first())],
    pad: 22,
    highlight: [
      { target: (p) => p.getByRole('switch', { name: /answers emails to this mailbox/ }), label: 1, inset: 5, badge: 'l' },
      { target: (p) => p.locator('#mailbox-bot'), label: 2, inset: 4, badge: 'l' },
    ],
  },
  {
    name: 'bots/thread',
    path: '/inbox/open',
    viewport: TALL,
    run: async (page) => {
      await page.getByText('SAML with Okta on the Team plan').first().click();
      await settle(page, 800);
    },
    clip: (p) => [p.getByText('How do I connect Okta for SAML sign-in?').last(), p.getByText('handed this to the team', { exact: false }).last()],
    pad: 20,
    highlight: [
      { target: (p) => p.getByText('Pip', { exact: true }).first().locator('xpath=..'), label: 1, inset: { x: 6, y: 3 }, badge: 'tr' },
      { target: (p) => p.getByText('Fetching an engineer for you now', { exact: false }).last(), label: 2, inset: 4, badge: 'l' },
    ],
  },
];
