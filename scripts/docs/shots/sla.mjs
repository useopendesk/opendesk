// Shots for docs/src/content/docs/guides/sla-policies.mdx.
import { around, block, button, labelled, settle } from '../helpers.mjs';

// A titled settings section: its heading, description and card.
const section = (p, title) => p.locator('section').filter({ has: p.locator('h2').filter({ hasText: new RegExp(`^\\s*${title}\\s*$`) }) }).first();
// A policy row in the list, by its name.
const policy = (p, name) => p.locator('main').getByText(name, { exact: true }).first().locator('xpath=ancestor::*[.//button[@role="switch"]][1]');

// A small pill inside a row, by its text.
const chip = (row, t) => row.locator('span').filter({ hasText: new RegExp(`^\\s*${t}\\s*$`) }).last();

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

async function sortByDue(page) {
  await labelled(page, 'Sort').click();
  await settle(page);
  await page.getByRole('menuitem', { name: 'SLA due' }).click();
  await page.keyboard.press('Escape');
  await settle(page, 500);
}

// The SLA block at the foot of a ticket's details panel.
const slaPanel = (p) => around(p, 'SLA', 'First response', 'Resolution').last();

async function openTeam(page, name) {
  await page.locator('main').getByText(name, { exact: true }).first().click();
  await settle(page, 600);
}

export const shots = [
  {
    name: 'sla/policies',
    path: '/settings/sla',
    clip: (p) => [p.locator('h1').last(), button(p, 'New policy'), policy(p, 'Standard')],
    pad: 28,
    highlight: [
      { target: (p) => button(p, 'New policy'), label: 1, badge: 'l' },
      { target: (p) => [policy(p, 'Urgent').getByText('1', { exact: true }), policy(p, 'Standard').getByText('5', { exact: true })], label: 2, inset: { x: 8, y: 6 }, badge: 'tl' },
      { target: (p) => chip(policy(p, 'Urgent'), 'Urgent'), label: 3, badge: 'tr' },
      { target: (p) => chip(policy(p, 'Legal requests'), 'Legal').locator('xpath=..'), label: 4, badge: 'tr' },
      { target: (p) => chip(policy(p, 'High priority'), 'Business hours'), label: 5, badge: 'tr' },
      { target: (p) => policy(p, 'Urgent').locator('button[role="switch"]'), label: 6, badge: 'l' },
    ],
  },
  {
    name: 'sla/new-policy',
    path: '/settings/sla',
    viewport: { width: 1440, height: 1000 },
    run: async (page) => {
      await openDialog(page, 'New policy', 'Urgent tickets');
      await page.getByPlaceholder('Urgent tickets').fill('Billing, high priority');
    },
    clip: (p) => p.getByRole('dialog'),
    pad: 24,
    highlight: [
      { target: (p) => [p.getByText('Applies to', { exact: true }), p.getByText('Applies to tickets of every team.')], label: 1, inset: 6 },
      { target: (p) => [p.getByText('First response within', { exact: true }), p.getByText(/^Currently 1d/)], label: 2, inset: 6 },
      { target: (p) => around(p, 'Count business hours only', 'The clock pauses').locator('xpath=..'), label: 3, inset: 2 },
    ],
  },
  {
    name: 'sla/business-hours',
    path: '/settings/business-hours',
    viewport: { width: 1440, height: 1240 },
    clip: (p) => [p.locator('h1').last(), section(p, 'Holidays')],
    pad: 28,
    highlight: [
      { target: (p) => around(p, 'Use business hours', 'Time zone', 'SLA policies').last(), label: 1, inset: 2 },
      { target: (p) => section(p, 'Weekly hours').locator('> div').last(), label: 2, inset: 2 },
      { target: (p) => section(p, 'Holidays'), label: 3, inset: 8 },
    ],
  },
  {
    name: 'sla/team-hours',
    path: '/settings/teams',
    viewport: { width: 1440, height: 1700 },
    run: async (page) => {
      await openTeam(page, 'Billing');
      await section(page, 'Hours').getByText('Own schedule').click();
      await settle(page);
    },
    clip: (p) => section(p, 'Hours'),
    pad: 16,
    highlight: [
      { target: (p) => section(p, 'Hours').getByText('Own schedule').locator('xpath=ancestor::*[contains(@class, "rounded")][2]'), label: 1, inset: 3, badge: 'tr' },
      { target: (p) => section(p, 'Hours').locator('> div').last(), label: 2, inset: 3, badge: 'bl' },
    ],
  },
  {
    name: 'sla/sort-due',
    path: '/inbox/open',
    run: async (page) => {
      await labelled(page, 'Sort').click();
      await settle(page);
    },
    keepFocus: true,
    clip: (p) => [labelled(p, 'Filter'), p.getByRole('menu')],
    pad: 24,
    highlight: [
      { target: (p) => labelled(p, 'Sort'), label: 1, badge: 'tr' },
      { target: (p) => p.getByRole('menuitem', { name: 'SLA due' }), label: 2, inset: 2, badge: 'r' },
    ],
  },
  {
    name: 'sla/due-list',
    path: '/inbox/open',
    run: sortByDue,
    viewport: { width: 1440, height: 760 },
    clip: () => [{ x: 241, y: 49, width: 1, height: 1 }, { x: 620, y: 700, width: 1, height: 1 }],
    pad: 0,
    highlight: [
      { target: (p) => labelled(p, 'Sort'), label: 1, badge: 'l' },
      { target: (p) => [p.getByText(/^\d+d \d+h over$/).first(), p.getByText(/^\d+h over$/).first()], label: 2, inset: 5, badge: 'l' },
    ],
  },
  {
    name: 'sla/ticket-sla',
    path: '/inbox/open',
    run: async (page) => {
      await sortByDue(page);
      await page.getByText('Imported Evernote notes lost formatting').first().click();
      await settle(page, 600);
    },
    viewport: { width: 1440, height: 1100 },
    clip: slaPanel,
    pad: 12,
  },
];
