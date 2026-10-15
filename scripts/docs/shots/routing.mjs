// Shots for docs/src/content/docs/guides/routing.mdx.
import { around, button, labelled, rows, settle, text } from '../helpers.mjs';

const main = (p) => p.locator('main');
const popover = (p) => p.locator('[data-radix-popper-content-wrapper]').filter({ visible: true }).last();
const testPanel = (p) => around(p, 'Write a sample email or chat', 'Message');
const flow = (p) => p.locator('ol[aria-label="How a new ticket gets its team"]');
const rulesList = (p) => around(p, 'Checked from the top for every new ticket', 'Contract questions');
const conditions = (p) => around(p, 'The conditions a new ticket is checked against.', 'Add a condition');

async function openRule(page, name) {
  await main(page).locator('a[href*="/settings/routing/"]').filter({ hasText: name }).first().click();
  await settle(page, 500);
}

async function openTicket(page, subject) {
  await page.getByText(subject, { exact: true }).first().click();
  await settle(page, 600);
}
const details = (p) => around(p, 'Status', 'Priority', 'Severity', 'Assignee', 'Labels');
const facts = (p) => around(p, 'Ticket', 'Channel', 'Received at', 'Created');

export const shots = [
  {
    name: 'routing/overview',
    path: '/settings/routing',
    viewport: { width: 1600, height: 900 },
    highlight: [
      { target: flow, label: 1, inset: 6 },
      { target: (p) => [labelled(p, 'Reorder Invoices to Billing'), labelled(p, 'Reorder Contract questions, Ollie decides')], label: 2, inset: 5, badge: 'l' },
      { target: (p) => main(p).locator('a[href*="/settings/routing/"]').filter({ hasText: 'Invoices to Billing' }).first(), label: 3, inset: 3 },
      { target: (p) => [labelled(p, 'Turn Invoices to Billing on or off'), labelled(p, 'Turn Contract questions, Ollie decides on or off')], label: 4, inset: 5, badge: 'tr' },
      { target: testPanel, label: 5, inset: 2 },
      { target: (p) => main(p).locator('a').filter({ hasText: /^\s*New rule\s*$/ }).first(), label: 6, badge: 'l' },
    ],
  },
  {
    name: 'routing/open-settings',
    path: '/settings/routing',
    dim: true,
    clip: () => [{ x: 0, y: 0, width: 1, height: 1 }, { x: 860, y: 899, width: 1, height: 1 }],
    pad: 0,
    highlight: [
      { target: (p) => p.getByRole('link', { name: 'Settings', exact: true }).last(), label: 1, badge: 'r' },
      { target: (p) => p.getByRole('link', { name: 'Routing', exact: true }).last(), label: 2, badge: 'r' },
    ],
  },
  {
    name: 'routing/rule',
    path: '/settings/routing',
    viewport: { width: 1440, height: 1300 },
    run: (page) => openRule(page, 'Invoices to Billing'),
    clip: (p) => [main(p).getByRole('heading', { name: 'Invoices to Billing' }).first(), around(p, 'Where matching tickets go.', 'Send to team')],
    pad: 24,
    highlight: [
      { target: (p) => around(p, 'Name', 'Checked for every new ticket.'), label: 1, inset: 3, badge: 'l' },
      { target: (p) => [main(p).getByText('Any matches', { exact: true }), main(p).getByText('All match', { exact: true })], label: 2, inset: 6, badge: 'l' },
      { target: (p) => [main(p).locator('button').filter({ hasText: /^\s*Subject\s*$/ }).first(), labelled(p, 'Remove condition 3')], label: 3, inset: 2, badge: 'tr' },
      { target: (p) => button(p, 'Add a condition'), label: 4, badge: 'r' },
      { target: (p) => main(p).locator('button').filter({ hasText: /^\s*Billing\s*$/ }).last(), label: 5, inset: 4, badge: 'l' },
    ],
  },
  {
    name: 'routing/fields',
    path: '/settings/routing',
    viewport: { width: 1440, height: 1300 },
    run: async (page) => {
      await openRule(page, 'Invoices to Billing');
      await main(page).locator('button').filter({ hasText: /^\s*Subject\s*$/ }).first().click();
      await settle(page);
    },
    keepFocus: true,
    clip: (p) => [main(p).locator('button').filter({ hasText: /^\s*Subject\s*$/ }).first(), popover(p)],
    pad: 24,
    highlight: [{ target: popover, inset: 3 }],
  },
  {
    name: 'routing/test-email',
    path: '/settings/routing',
    viewport: { width: 1600, height: 1300 },
    run: async (page) => {
      const panel = testPanel(page);
      await panel.getByLabel('From').fill('dana@northwind.io');
      await panel.getByLabel('Subject').fill('Refund for my last invoice');
      await panel.getByLabel('Message').fill('Hi, I was charged twice this month.');
      await button(page, 'Test').click();
      await settle(page, 700);
    },
    clip: (p) => around(p, 'Write a sample email or chat', 'Decided by'),
    pad: 24,
    highlight: [
      { target: (p) => [main(p).getByText('Email', { exact: true }).last(), main(p).getByText('Chat', { exact: true }).last()], label: 1, inset: 6, badge: 'l' },
      { target: (p) => button(p, 'Test'), label: 2, badge: 'r' },
      { target: (p) => around(p, 'Steps', 'Invoices to Billing').locator('ol'), label: 3, inset: 6, badge: 'l' },
      { target: (p) => p.locator('dl').filter({ hasText: 'Decided by' }), label: 4, inset: 4, badge: 'l' },
    ],
  },
  {
    name: 'routing/test-chat',
    path: '/settings/routing',
    viewport: { width: 1600, height: 1300 },
    run: async (page) => {
      const panel = testPanel(page);
      await panel.getByText('Chat', { exact: true }).click();
      await settle(page);
      await panel.locator('button').filter({ hasText: /^\s*Marketing site/ }).first().click();
      await settle(page);
      await popover(page).getByText('Docs', { exact: true }).click();
      await settle(page);
      await panel.locator('button').filter({ hasText: /^\s*No topic/ }).first().click();
      await settle(page);
      await popover(page).getByText('Report a vulnerability', { exact: true }).click();
      await settle(page);
      await panel.getByLabel('First message').fill('I think I found an XSS bug in shared notes.');
      await button(page, 'Test').click();
      await settle(page, 700);
    },
    clip: (p) => around(p, 'Write a sample email or chat', 'Decided by'),
    pad: 24,
    highlight: [
      { target: (p) => [testPanel(p).getByText('Widget', { exact: true }), testPanel(p).locator('button').filter({ hasText: /Report a vulnerability/ }).first()], label: 1, inset: 6, badge: 'l' },
      { target: (p) => around(p, 'Steps', 'Invoices to Billing').locator('ol'), label: 2, inset: 6, badge: 'l' },
      { target: (p) => p.locator('dl').filter({ hasText: 'Decided by' }), label: 3, inset: 4, badge: 'l' },
    ],
  },
  {
    name: 'routing/ticket-details',
    path: '/inbox/open',
    viewport: { width: 1440, height: 1500 },
    run: (page) => openTicket(page, 'Refund for a paid subscriber'),
    clip: (p) => [details(p).getByText('Status', { exact: true }), details(p).getByText('Labels', { exact: true })],
    pad: 20,
    highlight: [
      { target: (p) => rows(details(p), 'Team', 'Reply from'), label: 1, inset: 0, badge: 'l' },
    ],
  },
  {
    name: 'routing/ticket-facts',
    path: '/inbox/open',
    viewport: { width: 1440, height: 1500 },
    run: (page) => openTicket(page, 'Refund for a paid subscriber'),
    clip: (p) => [p.getByText('Details', { exact: true }).filter({ visible: true }).last(), facts(p).getByText('Created', { exact: true })],
    pad: 20,
    highlight: [
      { target: (p) => facts(p).getByText('Received at', { exact: true }).locator('xpath=..'), label: 1, inset: 4, badge: 'l' },
      { target: (p) => facts(p).getByText(/^Routed to Billing/).locator('xpath=..'), label: 2, inset: 4, badge: 'l' },
    ],
  },
  {
    name: 'routing/message-to',
    path: '/inbox/open',
    viewport: { width: 1440, height: 1000 },
    run: (page) => openTicket(page, 'Refund for a paid subscriber'),
    clip: (p) => around(p, 'Vera Berg', 'A reader paid twice'),
    pad: 20,
    highlight: [{ target: (p) => main(p).getByText('to hello@pressroom.app', { exact: true }).first(), label: 1, inset: 1, badge: 'tr' }],
  },
  {
    name: 'routing/replying-as',
    path: '/inbox/open',
    viewport: { width: 1440, height: 1000 },
    run: async (page) => {
      await openTicket(page, 'Pricing question');
      const chip = labelled(page, 'Change the address this reply goes out from');
      await chip.click();
      await settle(page);
      // The first click can land while the composer settles: open it if it stayed shut.
      if ((await chip.getAttribute('data-state')) !== 'open') {
        await chip.click();
        await settle(page);
      }
    },
    keepFocus: true,
    clip: (p) => [around(p, 'Replying as', 'Send'), popover(p)],
    pad: 20,
    highlight: [
      { target: (p) => labelled(p, 'Change the address this reply goes out from'), label: 1, inset: 3, badge: 'l' },
      { target: popover, label: 2, inset: 3, badge: 'l' },
    ],
  },
];
