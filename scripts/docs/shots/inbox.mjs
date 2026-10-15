// Shots for docs/src/content/docs/guides/inbox-and-tickets.mdx.
import { around, block, button, labelled, link, rows, settle, text } from '../helpers.mjs';

// An email ticket with notes, a teammate replying and a full details panel.
async function openTicket(page) {
  await page.getByText('SSO login loops back to sign-in page').first().click();
  await settle(page, 500);
}

const folders = (p) => [link(p, 'Unassigned'), link(p, 'Spam')];
const views = (p) => [link(p, 'Urgent'), link(p, 'Legal requests due')];
const composer = (p) => around(p, 'Replying as', 'Send');
const details = (p) => around(p, 'Status', 'Priority', 'Severity', 'Assignee', 'Labels');

async function draft(page) {
  await page.locator('[contenteditable="true"]').first().click();
  await page.keyboard.type('Thanks Dana, adding the new fingerprint now. I will confirm once it is live.');
}

export const shots = [
  {
    name: 'inbox/overview',
    path: '/inbox/open',
    run: openTicket,
    highlight: [
      { target: folders, label: 1, inset: 3 },
      { target: views, label: 2, inset: 3 },
      { target: (p) => around(p, 'Julia Novak', 'Hugo', 'Aisha Bello', 'Elena Rossi'), label: 3, inset: -3 },
      { target: (p) => [button(p, 'Solve'), labelled(p, 'Snooze'), labelled(p, 'More actions')], label: 4 },
      { target: (p) => around(p, 'Jonas Weber', 'Confirmed. Every assertion'), label: 5, inset: 2 },
      { target: composer, label: 6, inset: 2 },
      { target: details, label: 7, inset: -2 },
    ],
  },
  {
    name: 'inbox/open-ticket',
    path: '/inbox/open',
    run: openTicket,
    dim: true,
    highlight: [
      { target: (p) => p.locator('a').filter({ hasText: /^\s*All open\s*\d/ }).first(), label: 1, inset: 2 },
      { target: (p) => block(labelled(p, 'Select ticket 1072')), label: 2, inset: -3 },
    ],
  },
  {
    name: 'inbox/reply',
    path: '/inbox/open',
    run: async (page) => {
      await openTicket(page);
      await draft(page);
    },
    clip: composer,
    pad: 20,
    highlight: [
      { target: (p) => [composer(p).getByText('Reply', { exact: true }), composer(p).getByText('Note', { exact: true })], label: 1 },
      { target: (p) => [button(p, 'Send'), labelled(p, 'Send and set status')], label: 2 },
    ],
  },
  {
    name: 'inbox/details',
    path: '/inbox/open',
    run: openTicket,
    clip: details,
    pad: 28,
    highlight: [
      { target: (p) => rows(details(p), 'Status', 'Severity'), label: 1, inset: 0 },
      { target: (p) => rows(details(p), 'Assignee', 'Team'), label: 2, inset: 0 },
      { target: (p) => rows(details(p), 'Labels'), label: 3, inset: 0 },
    ],
  },
  {
    name: 'inbox/snooze',
    path: '/inbox/open',
    run: async (page) => {
      await openTicket(page);
      await labelled(page, 'Snooze').click();
      await settle(page);
    },
    clip: (p) => [button(p, 'Solve'), labelled(p, 'More actions'), around(p, 'Snooze until', 'Next week')],
    pad: 24,
    highlight: [
      { target: (p) => labelled(p, 'Snooze'), label: 1 },
      { target: (p) => around(p, 'Snooze until', 'In 1 hour', 'Next week'), label: 2, inset: 2 },
    ],
  },
  {
    name: 'inbox/more-actions',
    path: '/inbox/open',
    run: async (page) => {
      await openTicket(page);
      await labelled(page, 'More actions').click();
      await settle(page);
    },
    clip: (p) => [button(p, 'Solve'), labelled(p, 'Toggle details'), p.getByRole('menu')],
    pad: 24,
    highlight: [
      { target: (p) => labelled(p, 'More actions'), label: 1 },
      { target: (p) => [p.getByRole('menuitem', { name: /Merge/ }), p.getByRole('menuitem', { name: /Delete ticket/ })], label: 2, inset: 3 },
    ],
  },
  {
    name: 'inbox/bulk',
    path: '/inbox/open',
    run: async (page) => {
      await labelled(page, 'Select ticket 1093').click({ force: true });
      await labelled(page, 'Select ticket 1065').click({ force: true });
      await settle(page);
    },
    viewport: { width: 1440, height: 640 },
    clip: (p) => [labelled(p, 'Filter'), labelled(p, 'Bulk actions'), block(labelled(p, 'Select ticket 1097'))],
    pad: 0,
    highlight: [
      { target: (p) => [labelled(p, 'Select ticket 1093'), labelled(p, 'Select ticket 1065')], label: 1, inset: 6 },
      { target: (p) => labelled(p, 'Bulk actions'), label: 2, inset: 3 },
    ],
  },
  {
    name: 'inbox/presence',
    path: '/inbox/open',
    run: openTicket,
    clip: (p) => [around(p, 'is replying to this ticket right now'), composer(p)],
    pad: 20,
    highlight: [{ target: (p) => around(p, 'is replying to this ticket right now', 'Leave a note instead'), label: 1, inset: 2 }],
  },
  {
    name: 'inbox/filter',
    path: '/inbox/open',
    run: async (page) => {
      await labelled(page, 'Filter').click();
      await settle(page);
    },
    clip: (p) => [{ x: 0, y: 0, width: 1, height: 1 }, link(p, 'Unassigned'), labelled(p, 'Sort'), p.getByRole('menu')],
    pad: 20,
    highlight: [
      { target: (p) => labelled(p, 'Filter') },
      { target: (p) => p.getByRole('menu'), inset: 3 },
    ],
  },
  {
    name: 'inbox/search',
    path: '/inbox/open',
    run: async (page) => {
      await labelled(page, 'Search and commands').click();
      await settle(page);
      await page.keyboard.type('fingerprint');
      await settle(page, 600);
    },
    clip: (p) => p.getByRole('dialog'),
    pad: 20,
  },
];
