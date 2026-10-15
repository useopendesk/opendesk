// Shots for docs/src/content/docs/guides/labels-and-custom-fields.mdx.
import { around, button, labelled, settle, text } from '../helpers.mjs';

const section = (p, title) => p.locator('section').filter({ has: p.locator('h2').filter({ hasText: new RegExp(`^\\s*${title}\\s*$`) }) }).first();
// A label group on the Labels page: its header row and its table.
const groupHead = (p, name) => p.locator('main').getByText(name, { exact: true }).first().locator('xpath=..');
const labelRow = (p, name) => labelled(p, `Actions for ${name}`).locator('xpath=ancestor::*[.//text()[normalize-space()="' + name + '"]][1]');

async function openDialog(page, name, placeholder) {
  for (let i = 0; i < 3; i++) {
    await button(page, name).click();
    try {
      await page.getByPlaceholder(placeholder).first().waitFor({ timeout: 4000 });
      break;
    } catch {}
  }
  await settle(page);
}

async function openTicket(page) {
  await page.getByText('SSO login loops back to sign-in page').first().click();
  await settle(page, 800);
}


export const shots = [
  {
    name: 'labels/labels',
    path: '/settings/tags',
    viewport: { width: 1440, height: 1000 },
    clip: (p) => [p.locator('h1').last(), button(p, 'New label'), labelRow(p, 'Billing')],
    pad: 28,
    highlight: [
      { target: (p) => [button(p, 'New group'), button(p, 'New label')], label: 1, inset: 5, badge: 'l' },
      { target: (p) => p.getByText('One at a time', { exact: true }).first(), label: 2, inset: 4, badge: 'tr' },
      { target: (p) => labelRow(p, 'Question'), label: 3, inset: -2, badge: 'bl' },
      { target: (p) => [p.getByText('Applies to', { exact: true }).first(), p.getByText('Uses', { exact: true }).first()], label: 4, inset: 6, badge: 'tr' },
    ],
  },
  {
    name: 'labels/new-label',
    path: '/settings/tags',
    viewport: { width: 1440, height: 1000 },
    run: async (page) => {
      await openDialog(page, 'New label', 'billing');
      await page.getByPlaceholder('billing').fill('Refund request');
      await page.getByPlaceholder('Questions about invoices, refunds and payment methods').fill('A customer asks for money back');
    },
    clip: (p) => p.getByRole('dialog'),
    pad: 24,
    highlight: [
      { target: (p) => [text(p, 'Name'), p.getByPlaceholder('billing')], label: 1, inset: 6, badge: 'l' },
      { target: (p) => [text(p, 'Description', false), text(p, 'Shown in pickers so everyone uses the label the same way.')], label: 2, inset: 6, badge: 'l' },
      { target: (p) => [text(p, 'Group'), text(p, 'No group')], label: 3, inset: 6, badge: 'l' },
      { target: (p) => around(p, 'Applies to', 'Tickets', 'Contacts', 'Companies').last(), label: 4, inset: 6, badge: 'l' },
    ],
  },
  {
    name: 'labels/label-menu',
    path: '/settings/tags',
    run: async (page) => {
      await labelled(page, 'Actions for Bug').click();
      await settle(page);
    },
    keepFocus: true,
    clip: (p) => [labelRow(p, 'Bug'), p.getByRole('menu')],
    pad: 20,
    highlight: [
      { target: (p) => [p.getByRole('menuitem', { name: /Merge/ }), p.getByRole('menuitem', { name: /Archive/ })], label: 1, inset: 3, badge: 'l' },
    ],
  },
  {
    name: 'labels/creation',
    path: '/settings/tags',
    viewport: { width: 1440, height: 2600 },
    clip: (p) => section(p, 'Creating labels'),
    pad: 24,
    highlight: [{ target: (p) => around(p, 'Anyone', 'Admins only').last(), label: 1, inset: 3, badge: 'l' }],
  },
  {
    name: 'labels/ticket-labels',
    path: '/inbox/open',
    run: async (page) => {
      await openTicket(page);
      await page.locator('aside, main').getByText('Add', { exact: true }).last().click();
      await settle(page);
    },
    keepFocus: true,
    clip: (p) => [text(p, 'Reply from'), p.locator('[data-radix-popper-content-wrapper]').last()],
    pad: 20,
    highlight: [
      { target: (p) => p.locator('aside, main').getByText('Add', { exact: true }).last(), label: 1, badge: 'r' },
      { target: (p) => p.getByText('Type, pick one', { exact: true }), label: 2, inset: { x: 4, y: 1 }, badge: 'r' },
    ],
  },
  {
    name: 'labels/fields',
    path: '/settings/custom-fields',
    viewport: { width: 1440, height: 1000 },
    clip: (p) => [p.locator('h1').last(), button(p, 'New field'), p.getByText('Refund amount (EUR)')],
    pad: 28,
    highlight: [
      { target: (p) => around(p, 'Tickets', 'Contacts', 'Companies').last(), label: 1, inset: 3, badge: 'l' },
      { target: (p) => button(p, 'New field'), label: 2, badge: 'l' },
      { target: (p) => p.getByText('Required to solve', { exact: true }).first(), label: 3, inset: 4, badge: 'tr' },
      { target: (p) => p.getByText('Archived', { exact: true }).last(), label: 4, inset: 4, badge: 'tr' },
    ],
  },
  {
    name: 'labels/new-field',
    path: '/settings/custom-fields',
    viewport: { width: 1440, height: 1100 },
    run: async (page) => {
      await openDialog(page, 'New field', 'Plan tier');
      await page.getByPlaceholder('Plan tier').fill('Order number');
    },
    clip: (p) => p.getByRole('dialog'),
    pad: 24,
  },
  {
    name: 'labels/ticket-fields',
    path: '/inbox/open',
    viewport: { width: 1440, height: 1000 },
    run: openTicket,
    clip: (p) => [text(p, 'Labels'), p.getByText(/^Show \d+ empty fields$/).first(), p.locator('aside, main').getByText('Add', { exact: true }).last()],
    pad: 20,
    highlight: [
      { target: (p) => [p.getByText('Resolution', { exact: true }).first(), p.getByText('Empty', { exact: true }).first()], label: 1, inset: 5, badge: 'tr' },
    ],
  },
];
