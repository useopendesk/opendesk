// Shots for docs/src/content/docs/guides/teams-and-roles.mdx.
import { around, button, labelled, settle, text } from '../helpers.mjs';

const main = (p) => p.locator('main');
const dialog = (p) => p.locator('[role="dialog"], [role="alertdialog"]').filter({ visible: true }).last();
const teamRow = (p, name) => main(p).locator('a[href*="/settings/teams/"]').filter({ hasText: name }).first();

async function openTeam(page, name) {
  await teamRow(page, name).click();
  await settle(page, 500);
}

// The team page is long: a tall window keeps every section on screen.
const tall = { width: 1440, height: 2400 };

const membersSection = (p) => around(p, 'Who is on the team, and what they do on it.', 'Theo Brandt');
const hoursSection = (p) => around(p, 'Used for SLA timers on the team', 'Time zone');
const assignSection = (p) => around(p, 'Who gets the team', 'Assign new tickets', 'Which rule wins');
const signatureSection = (p) => around(p, 'Added to replies on the team', 'Variables');
const addressesSection = (p) => around(p, 'Email to these addresses goes to this team.', 'Also routed here');

// An email ticket of Support that arrived at support@margin.app.
async function openSupportTicket(page) {
  await page.getByText('Rename workspace URL', { exact: true }).first().click();
  await settle(page, 500);
}
const details = (p) => around(p, 'Status', 'Priority', 'Severity', 'Assignee', 'Labels');
const popover = (p) => p.locator('[data-radix-popper-content-wrapper]').filter({ visible: true }).last();

export const shots = [
  {
    name: 'teams/overview',
    path: '/settings/teams',
    highlight: [
      { target: (p) => button(p, 'New team'), label: 1, badge: 'l' },
      { target: (p) => teamRow(p, 'Support'), label: 2, inset: -2 },
      { target: (p) => teamRow(p, 'Legal').getByText('Private', { exact: true }), label: 3, badge: 'tr' },
      { target: (p) => [main(p).getByText('People', { exact: true }), teamRow(p, 'Partnerships').locator('span').filter({ hasText: /^SO$/ }).first()], label: 4, inset: 6 },
      { target: (p) => [main(p).getByText('Open', { exact: true }).last(), main(p).getByText('Unassigned', { exact: true }).last()], label: 5, inset: 6 },
    ],
  },
  {
    name: 'teams/open-settings',
    path: '/settings/teams',
    dim: true,
    clip: () => [{ x: 0, y: 0, width: 1, height: 1 }, { x: 860, y: 899, width: 1, height: 1 }],
    pad: 0,
    highlight: [
      { target: (p) => p.getByRole('link', { name: 'Settings', exact: true }).last(), label: 1, badge: 'r' },
      { target: (p) => p.getByRole('link', { name: 'Teams', exact: true }).last(), label: 2, badge: 'r' },
    ],
  },
  {
    name: 'teams/new-team',
    path: '/settings/teams',
    run: async (page) => {
      await button(page, 'New team').click();
      await settle(page);
      await dialog(page).getByLabel('Name').fill('Enterprise');
      await dialog(page).getByLabel('Description').fill('Onboarding, contracts and renewals for Enterprise customers');
    },
    clip: dialog,
    pad: 28,
    highlight: [
      { target: (p) => [dialog(p).getByLabel('Name'), dialog(p).getByLabel('Description')], label: 1, inset: 6, badge: 'l' },
      { target: (p) => around(p, 'Colour').locator('[role="radiogroup"], div').last(), label: 2, inset: 5, badge: 'l' },
      { target: (p) => dialog(p).locator('label').filter({ hasText: 'Add me as the lead' }), label: 3, inset: 0, badge: 'l' },
      { target: (p) => dialog(p).locator('label').filter({ hasText: 'Private team' }), label: 4, inset: 0, badge: 'l' },
    ],
  },
  {
    name: 'teams/members',
    path: '/settings/teams',
    viewport: tall,
    run: (page) => openTeam(page, 'Support'),
    clip: membersSection,
    pad: 28,
    highlight: [
      { target: (p) => around(p, 'Manages the team', 'Sees the team'), label: 1, inset: -2 },
      { target: (p) => button(p, 'Add people'), label: 2, badge: 'l' },
      { target: (p) => labelled(p, 'Role for Theo Brandt'), label: 3, badge: 'l' },
    ],
  },
  {
    name: 'teams/private-join',
    path: '/settings/teams',
    viewport: { width: 1440, height: 1100 },
    run: (page) => openTeam(page, 'Legal'),
    clip: (p) => [main(p).getByText('Contracts, DPAs', { exact: false }).first(), button(p, 'Open inbox'), around(p, 'Legal is private.').locator('xpath=..')],
    pad: 24,
    highlight: [
      { target: (p) => button(p, 'Join to see tickets'), label: 1, badge: 'l' },
      { target: (p) => around(p, 'Legal is private.').locator('xpath=..'), label: 2, inset: 3 },
    ],
  },
  {
    name: 'teams/privacy',
    path: '/settings/teams',
    viewport: tall,
    run: (page) => openTeam(page, 'Legal'),
    clip: (p) => around(p, 'Who sees the team', 'Admins are not exempt'),
    pad: 24,
    highlight: [
      { target: (p) => labelled(p, 'Private team'), label: 1, badge: 'l' },
      { target: (p) => text(p, 'Admins are not exempt', false).locator('xpath=..'), label: 2, inset: 2 },
    ],
  },
  {
    name: 'teams/join-confirm',
    path: '/settings/teams',
    run: async (page) => {
      await openTeam(page, 'Legal');
      await button(page, 'Join to see tickets').click();
      await settle(page);
    },
    clip: dialog,
    pad: 24,
    keepFocus: true,
  },
  {
    name: 'teams/hours',
    path: '/settings/teams',
    viewport: tall,
    run: async (page) => {
      await openTeam(page, 'Support');
      await main(page).getByText('Own schedule', { exact: true }).click();
      await settle(page);
    },
    clip: hoursSection,
    pad: 24,
    highlight: [
      { target: (p) => [main(p).getByText('Workspace hours', { exact: true }), main(p).getByText('Own schedule', { exact: true })], label: 1, inset: 6, badge: 'l' },
      { target: (p) => around(p, 'Time zone').locator('xpath=..'), label: 2, inset: -2 },
    ],
  },
  {
    name: 'teams/assignment',
    path: '/settings/teams',
    viewport: tall,
    run: (page) => openTeam(page, 'Support'),
    clip: assignSection,
    pad: 24,
    highlight: [
      { target: (p) => labelled(p, 'Assign new tickets'), label: 1, badge: 'l' },
      { target: (p) => button(p, 'Assignment settings'), label: 2, badge: 'l' },
    ],
  },
  {
    name: 'teams/signature',
    path: '/settings/teams',
    viewport: tall,
    run: (page) => openTeam(page, 'Support'),
    clip: signatureSection,
    pad: 24,
    highlight: [
      { target: (p) => signatureSection(p).locator('[contenteditable="true"]'), label: 1, inset: 4 },
      { target: (p) => around(p, 'Variables', '{{agent.name}}', '{{contact.name}}'), label: 2, inset: { x: 8, y: 2 } },
    ],
  },
  {
    name: 'teams/addresses',
    path: '/settings/teams',
    viewport: tall,
    run: (page) => openTeam(page, 'Support'),
    clip: addressesSection,
    pad: 24,
    highlight: [
      { target: (p) => button(p, 'Add an address'), label: 1, badge: 'l' },
      { target: (p) => [main(p).locator('a[href*="/settings/mailboxes/"]').filter({ hasText: 'support@margin.app' }).first(), main(p).locator('a[href*="/settings/mailboxes/"]').filter({ hasText: 'hello@pressroom.app' }).first()], label: 2, inset: -3 },
      { target: (p) => around(p, 'Also routed here'), label: 3, inset: -2 },
    ],
  },
  {
    name: 'teams/delete',
    path: '/settings/teams',
    viewport: tall,
    run: async (page) => {
      await openTeam(page, 'Partnerships');
      await button(page, 'Delete team').click();
      await settle(page);
    },
    clip: dialog,
    pad: 28,
    keepFocus: true,
    highlight: [
      { target: (p) => dialog(p).locator('ul').first(), label: 1, inset: 4 },
      { target: (p) => around(p, 'Move them to', 'go to Support'), label: 2, inset: 4 },
      { target: (p) => around(p, 'Type Partnerships to confirm'), label: 3, inset: 4 },
    ],
  },
  {
    name: 'teams/move-ticket',
    path: '/inbox/open',
    viewport: { width: 1440, height: 1000 },
    run: async (page) => {
      await openSupportTicket(page);
      await details(page).getByText('Support', { exact: true }).first().click();
      await settle(page);
      await page.keyboard.type('Billing');
      await page.keyboard.press('Enter');
      await settle(page);
    },
    keepFocus: true,
    clip: (p) => [details(p).getByText('Status', { exact: true }), popover(p)],
    pad: 24,
    highlight: [
      { target: (p) => popover(p).locator('li').filter({ hasText: 'Replies go from' }), label: 1, inset: 4 },
      { target: (p) => popover(p).locator('button').filter({ hasText: /^\s*Move\s*$/ }), label: 2, badge: 'bl' },
    ],
  },
  {
    name: 'teams/team-inbox',
    path: '/settings/teams',
    run: async (page) => {
      await page.locator('aside a, nav a').filter({ hasText: /^\s*Billing\s*\d/ }).first().click();
      await settle(page, 500);
    },
    clip: () => [{ x: 0, y: 50, width: 1, height: 1 }, { x: 620, y: 620, width: 1, height: 1 }],
    pad: 0,
    highlight: [
      { target: (p) => [p.locator('a').filter({ hasText: /^\s*My teams/ }).first(), p.locator('a').filter({ hasText: /^\s*Support\s*\d/ }).first()], label: 1, inset: 4 },
      { target: (p) => [p.getByRole('tab', { name: /Unassigned/ }).first(), p.getByRole('tab', { name: /^All/ }).first()], label: 2, inset: 5 },
    ],
  },
  {
    name: 'teams/api-key',
    path: '/settings/api-keys',
    viewport: { width: 1440, height: 1100 },
    run: async (page) => {
      await button(page, 'New key').click();
      await settle(page);
    },
    keepFocus: true,
    clip: dialog,
    pad: 24,
    highlight: [{ target: (p) => around(p, 'Private teams', 'Tickets of private teams are left out'), label: 1, inset: 8 }],
  },
  {
    name: 'teams/scoped-sla',
    path: '/settings/sla',
    run: async (page) => {
      await main(page).getByText('Legal requests', { exact: true }).first().click();
      await settle(page);
    },
    keepFocus: true,
    clip: (p) => [dialog(p).getByText('Edit SLA policy', { exact: true }), around(p, 'Applies only to this team')],
    pad: 24,
    highlight: [{ target: (p) => around(p, 'Team', 'Applies only to this team'), label: 1, inset: 8 }],
  },
  {
    name: 'teams/scoped-reply',
    path: '/settings/saved-replies',
    run: async (page) => {
      await main(page).getByText('Refund processed', { exact: true }).first().click();
      await settle(page);
    },
    clip: (p) => [main(p).getByText('Refund processed', { exact: true }).first(), around(p, 'Offered on').locator('xpath=..')],
    pad: 24,
    highlight: [
      { target: (p) => around(p, 'Offered on').locator('xpath=..'), label: 1, inset: 4 },
      { target: (p) => main(p).getByText('/refund', { exact: true }).first().locator('xpath=..'), label: 2, inset: 4, badge: 'l' },
    ],
  },
  {
    name: 'teams/scoped-view',
    path: '/settings/views',
    run: async (page) => {
      await main(page).getByText('Legal requests due', { exact: true }).first().click();
      await settle(page);
    },
    keepFocus: true,
    clip: (p) => [dialog(p).getByText('Edit view', { exact: true }).filter({ visible: true }).first(), around(p, 'Who sees this view', 'Everyone on the team sees it')],
    pad: 24,
    highlight: [{ target: (p) => around(p, 'Who sees this view', 'Everyone on the team sees it'), label: 1, inset: 2 }],
  },
];
