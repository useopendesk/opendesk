// Shots for docs/src/content/docs/guides/reports.mdx: the reports page and its date range.
import { settle } from '../helpers.mjs';
import { framed } from './ai.mjs';

const card = (p, t) => framed(p.getByText(t, { exact: true }).first());
const tile = (p, t) => framed(p.locator('main').getByText(t, { exact: true }).first(), 100);
const item = (p, t) => p.getByText(t, { exact: true }).last().locator('xpath=ancestor-or-self::*[self::button or @role="option" or @role="menuitem" or @role="menuitemradio"][1]');
const range = (p) => p.locator('main button').filter({ hasText: 'Last 30 days' }).first();
const teams = (p) => p.locator('main button').filter({ hasText: 'All teams' }).first();
const brands = (p) => p.locator('main button').filter({ hasText: 'All brands' }).first();
// The range popover: the smallest box around the presets and the Apply button.
const menu = (p) => [p.getByText('Last 7 days', { exact: true }).last(), p.getByText('Apply range', { exact: true }).last()];

export const shots = [
  {
    name: 'reports/overview',
    path: '/reports',
    viewport: { width: 1440, height: 1760 },
    highlight: [
      { target: (p) => [teams(p), range(p)], label: 1, inset: 3, badge: 'bl' },
      { target: (p) => [tile(p, 'Created'), tile(p, 'SLA breached')], label: 2, inset: 5, badge: 'tl' },
      { target: (p) => card(p, 'Volume over time'), label: 3, inset: 5, badge: 'tl' },
      { target: (p) => card(p, 'Backlog'), label: 4, inset: 5, badge: 'tr' },
      { target: (p) => [card(p, 'By channel'), card(p, 'By label')], label: 5, inset: 5, badge: 'tl' },
      { target: (p) => [card(p, 'By team'), card(p, 'By brand')], label: 6, inset: 5, badge: 'tl' },
      { target: (p) => [card(p, 'Leaderboard'), card(p, 'Response times')], label: 7, inset: 5, badge: 'tl' },
    ],
  },
  {
    name: 'reports/teams',
    path: '/reports',
    run: async (page) => {
      await teams(page).click();
      await settle(page, 400);
    },
    clip: (p) => [teams(p), brands(p), p.getByPlaceholder('Search teams').last(), p.getByText('No team', { exact: true }).last()],
    pad: 22,
    highlight: [
      { target: (p) => [teams(p), brands(p)], label: 1, inset: 3, badge: 'l' },
      { target: (p) => item(p, 'Support'), label: 2, inset: 2, badge: 'l' },
    ],
  },
  {
    name: 'reports/range',
    path: '/reports',
    run: async (page) => {
      await range(page).click();
      await settle(page, 400);
    },
    clip: (p) => [range(p), ...menu(p)],
    pad: 22,
    highlight: [
      { target: (p) => [item(p, 'Last 7 days'), item(p, 'Last month')], label: 1, inset: 2, badge: 'l' },
      { target: (p) => [p.getByText(/^custom range$/i).last(), p.getByText('Apply range', { exact: true }).last()], label: 2, inset: { x: 8, y: 8 }, badge: 'l' },
    ],
  },
  {
    name: 'reports/kpis',
    path: '/reports',
    clip: (p) => [tile(p, 'Created'), tile(p, 'SLA breached')],
    pad: 12,
    highlight: [
      { target: (p) => p.getByText('↑5%', { exact: false }).first(), label: 1, inset: 4, badge: 'bl' },
      { target: (p) => tile(p, 'First response'), label: 2, inset: 3, badge: 'tl' },
    ],
  },
  {
    name: 'reports/volume',
    path: '/reports',
    run: async (page) => {
      await page.locator('main').getByText('Table', { exact: true }).first().click();
      await settle(page, 400);
    },
    clip: (p) => card(p, 'Volume over time'),
    pad: 16,
    highlight: [
      { target: (p) => [p.locator('main').getByText('Chart', { exact: true }).first(), p.locator('main').getByText('Table', { exact: true }).first()], label: 1, inset: { x: 12, y: 8 }, badge: 'bl' },
      { target: (p) => [p.getByText('Daily average', { exact: true }).first(), p.getByText('Solve rate', { exact: true }).first(), p.getByText('solved of created', { exact: false }).first()], label: 2, inset: { x: 12, y: 8 }, badge: 'tl' },
    ],
  },
];
