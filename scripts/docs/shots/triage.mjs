// Shots for docs/src/content/docs/guides/triage-and-assignment.mdx.
import { around, button, settle, text } from '../helpers.mjs';

// A titled settings section (its heading, description and card), by its heading.
const section = (p, title) => p.locator('section').filter({ has: p.locator('h2').filter({ hasText: new RegExp(`^\\s*${title}\\s*$`) }) }).first();
// The card of a section, without its heading.
const card = (p, title) => section(p, title).locator('> div').last();
const heading = (p, title) => section(p, title).locator('h2').first();

async function openTeam(page, name) {
  await page.locator('main').getByText(name, { exact: true }).first().click();
  await settle(page, 600);
}

export const shots = [
  {
    name: 'triage/assignment',
    path: '/settings/assignment',
    viewport: { width: 1440, height: 1160 },
    clip: (p) => [p.locator('h1').last(), section(p, 'Away')],
    pad: 28,
    highlight: [
      { target: (p) => card(p, 'Default mode'), label: 1, inset: 4 },
      { target: (p) => around(p, "The ticket's team", 'The address', 'Workspace default').last(), label: 2, inset: { x: 10, y: 6 } },
      { target: (p) => card(p, 'Rules'), label: 3, inset: 4 },
      { target: (p) => card(p, 'Away'), label: 4, inset: 4 },
    ],
  },
  {
    name: 'triage/availability',
    path: '/settings/assignment',
    viewport: { width: 1440, height: 2200 },
    clip: (p) => [heading(p, 'Team availability'), around(p, 'By team', 'Everyone').last(), section(p, 'Team availability').getByText('Cap 12').first()],
    pad: 24,
    highlight: [
      { target: (p) => around(p, 'By team', 'Everyone').last(), label: 1, badge: 'tr' },
      { target: (p) => section(p, 'Team availability').getByText(/^(Online|Away|Offline)$/).first(), label: 2, inset: 6 },
      { target: (p) => section(p, 'Team availability').getByText('Set cap').first(), label: 3, inset: 6, badge: 'tr' },
    ],
  },
  {
    name: 'triage/my-status',
    path: '/inbox/open',
    run: async (page) => {
      await page.getByText('maya@margin.app').last().click();
      await settle(page);
    },
    keepFocus: true,
    clip: () => [{ x: 0, y: 420, width: 1, height: 1 }, { x: 420, y: 899, width: 1, height: 1 }],
    pad: 0,
    highlight: [
      { target: (p) => around(p, 'Availability', 'Online', 'Away', 'Offline').last(), label: 1, inset: 2, badge: 'r' },
      { target: (p) => text(p, 'Maya Chen').last().locator('xpath=ancestor::button[1]'), label: 2, inset: 2, badge: 'r' },
    ],
  },
  {
    name: 'triage/team-members',
    path: '/settings/teams',
    viewport: { width: 1440, height: 1500 },
    run: (page) => openTeam(page, 'Support'),
    clip: (p) => section(p, 'Members'),
    pad: 24,
    highlight: [
      { target: (p) => around(p, 'Lead', 'Member', 'Observer', 'is never assigned').last(), label: 1, inset: 3 },
      { target: (p) => section(p, 'Members').locator('button').filter({ hasText: /^\s*Observer\s*$/ }).first(), label: 2, inset: 4, badge: 'l' },
    ],
  },
  {
    name: 'triage/team-settings',
    path: '/settings/teams',
    viewport: { width: 1440, height: 2000 },
    run: (page) => openTeam(page, 'Support'),
    clip: (p) => [section(p, 'General'), heading(p, 'Members')],
    pad: 24,
    highlight: [{ target: (p) => [text(p, 'Description'), p.locator('main textarea').first()], label: 1, inset: 6 }],
  },
  {
    name: 'triage/team-assignment',
    path: '/settings/teams',
    viewport: { width: 1440, height: 2000 },
    run: (page) => openTeam(page, 'Support'),
    clip: (p) => section(p, 'Assignment'),
    pad: 24,
    highlight: [{ target: (p) => section(p, 'Assignment').getByText('Round robin', { exact: true }).first().locator('xpath=ancestor::button[1]'), label: 1, inset: 4, badge: 'l' }],
  },
  {
    name: 'triage/settings',
    path: '/settings/triage',
    viewport: { width: 1440, height: 1240 },
    clip: (p) => [p.locator('h1').last(), section(p, 'After triage')],
    pad: 28,
    highlight: [
      { target: (p) => card(p, 'Decision engine'), label: 1, inset: 4 },
      { target: (p) => section(p, 'Triage').locator('button[role="switch"]').first(), label: 2, inset: 4, badge: 'l' },
      { target: (p) => around(p, 'What Ollie may change', 'Always recorded').last(), label: 3, inset: { x: 10, y: 4 }, badge: 'tr' },
      { target: (p) => card(p, 'After triage'), label: 4, inset: 4 },
    ],
  },
];
