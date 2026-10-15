// Shots for docs/src/content/docs/guides/integrations.mdx: connecting products over MCP,
// choosing tools, approvals that run in another product, and ticket links.
import { button, settle } from '../helpers.mjs';
import { framed } from './ai.mjs';

const main = (p) => p.locator('main');
const dialog = (p) => p.locator('[role="dialog"]').filter({ visible: true }).last();
const linear = (p) => framed(main(p).getByText('mcp.linear.app', { exact: true }).first(), 400);
const catalogCard = (p, name) => framed(main(p).getByText(name, { exact: true }).last());
const olliePanel = (p) => p.locator('section[aria-label="Ollie"]').first();
const panel = (p) => p.locator('[data-ollie-panel]').first();
const approval = (p) => framed(panel(p).getByText('Runs in Linear', { exact: true }).first(), 300);

async function connect(page, name) {
  const card = main(page).getByText(name, { exact: true }).last().locator('xpath=ancestor::*[.//button][1]');
  await card.getByRole('button', { name: 'Connect' }).click();
  await settle(page, 500);
}

export const shots = [
  {
    name: 'integrations/overview',
    path: '/settings/integrations',
    viewport: { width: 1440, height: 1000 },
    highlight: [
      { target: (p) => button(p, 'Add by URL'), label: 1, inset: 3, badge: 'bl' },
      { target: linear, label: 2, inset: 4, badge: 'tl' },
      { target: (p) => [main(p).getByText('All', { exact: true }).first(), main(p).getByText('Automation', { exact: true }).first()], label: 3, inset: { x: 8, y: 6 }, badge: 'bl' },
      { target: (p) => catalogCard(p, 'GitHub'), label: 4, inset: 4, badge: 'tl' },
    ],
  },
  {
    name: 'integrations/tools',
    path: '/settings/integrations',
    viewport: { width: 1440, height: 1100 },
    run: async (page) => {
      await main(page).getByText('mcp.linear.app', { exact: true }).first().click();
      await settle(page, 600);
    },
    clip: linear,
    pad: 16,
    highlight: [
      { target: (p) => main(p).getByText('Looks up', { exact: true }).first(), label: 1, inset: 4, badge: 'r' },
      { target: (p) => main(p).getByText('Asks first', { exact: true }).first(), label: 2, inset: 4, badge: 'r' },
      { target: (p) => main(p).getByRole('switch').nth(1), label: 3, inset: 4, badge: 'l' },
      { target: (p) => [main(p).getByRole('switch').first(), p.locator('button[aria-label="Actions for Linear"]').first()], label: 4, inset: 4, badge: 'tl' },
    ],
  },
  {
    name: 'integrations/menu',
    path: '/settings/integrations',
    run: async (page) => {
      await page.locator('button[aria-label="Actions for Linear"]').first().click();
      await settle(page, 400);
    },
    clip: (p) => [p.locator('button[aria-label="Toggle Linear"], main button[role="switch"]').first(), p.locator('[role="menu"]').last()],
    pad: 22,
    highlight: [{ target: (p) => p.locator('[role="menu"]').last(), label: 1, inset: 3, badge: 'bl' }],
  },
  {
    name: 'integrations/token',
    path: '/settings/integrations',
    run: (page) => connect(page, 'GitHub'),
    clip: dialog,
    pad: 20,
    dim: true,
    highlight: [
      { target: (p) => [dialog(p).getByText('Access token', { exact: true }), dialog(p).locator('input').first(), dialog(p).getByText('Create one', { exact: false })], label: 1, inset: { x: 10, y: 6 }, badge: 'r' },
      { target: (p) => dialog(p).getByText('Ollie only looks things up', { exact: false }), label: 2, inset: 6, badge: 'r' },
    ],
  },
  {
    name: 'integrations/url',
    path: '/settings/integrations',
    run: async (page) => {
      await button(page, 'Add by URL').click();
      await settle(page, 500);
      await dialog(page).locator('input').first().fill('https://mcp.internal-tools.example.com/mcp');
      await dialog(page).locator('input').nth(1).fill('Internal tools');
    },
    clip: dialog,
    pad: 20,
    dim: true,
    highlight: [
      { target: (p) => [dialog(p).getByText('Server URL', { exact: true }), dialog(p).getByText('Usually ends in', { exact: false })], label: 1, inset: { x: 10, y: 6 }, badge: 'r' },
      { target: (p) => [dialog(p).getByText('Sign-in', { exact: true }), dialog(p).getByText('No sign-in', { exact: true })], label: 2, inset: { x: 10, y: 6 }, badge: 'r' },
      { target: (p) => dialog(p).locator('button').filter({ hasText: 'Continue to the server' }), label: 3, inset: 3, badge: 'bl' },
    ],
  },
  {
    name: 'integrations/approval',
    path: '/inbox/open',
    viewport: { width: 1440, height: 1100 },
    run: async (page) => {
      await page.getByText('SSO login loops back to sign-in page').first().click();
      await settle(page, 600);
      await olliePanel(page).getByText('Create a Linear issue', { exact: true }).first().click();
      await page.waitForTimeout(8000);
    },
    clip: (p) => [panel(p).getByText('Read the ticket', { exact: true }).first(), approval(p)],
    pad: 20,
    highlight: [
      { target: (p) => panel(p).getByText('Runs in Linear', { exact: true }).first(), label: 1, inset: 4, badge: 'tl' },
      { target: (p) => framed(panel(p).getByText('Title', { exact: true }).first(), 200), label: 2, inset: 3, badge: 'l' },
      { target: (p) => [panel(p).getByRole('button', { name: 'Deny' }), panel(p).locator('button').filter({ hasText: /^\s*Approve/ }).first()], label: 3, inset: 3, badge: 'bl' },
    ],
  },
  {
    name: 'integrations/linked',
    path: '/inbox/open',
    viewport: { width: 1440, height: 1300 },
    run: async (page) => {
      await page.getByText('Mobile app crashes when opening shared notebook', { exact: true }).first().click();
      await settle(page, 700);
    },
    clip: (p) => [olliePanel(p), p.getByText('Linear', { exact: true }).last()],
    pad: 12,
    highlight: [
      { target: (p) => [olliePanel(p).getByText('In your tools', { exact: true }).first(), olliePanel(p).getByText('Find related Linear issues', { exact: true }).first()], label: 1, inset: { x: 9, y: 7 }, badge: 'tr' },
      { target: (p) => [p.getByText('Linked', { exact: true }).last(), p.getByText('Crash when opening shared', { exact: false }).last(), p.getByText('Linear', { exact: true }).last()], label: 2, inset: { x: 10, y: 8 }, badge: 'tr' },
    ],
  },
];
