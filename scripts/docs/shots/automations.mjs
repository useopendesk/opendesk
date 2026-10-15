// Shots for docs/src/content/docs/guides/automations.mdx.
import { around, button, settle } from '../helpers.mjs';

// A step of the automation editor ("When", "If all of these match", "Then"): its card.
const step = (p, title) => p.locator('main').getByText(title, { exact: true }).first().locator('xpath=ancestor::div[.//div[contains(@class, "rounded")]][1]');
const row = (p, name) => p.locator('main').getByText(name, { exact: true }).first().locator('xpath=ancestor::*[.//button[@role="switch"]][1]');
const popover = (p) => p.locator('[data-radix-popper-content-wrapper]').last();

async function openAutomation(page, name) {
  for (let i = 0; i < 3; i++) {
    await page.getByText(name, { exact: true }).first().click();
    try {
      await page.getByText('If all of these match').waitFor({ timeout: 4000 });
      break;
    } catch {}
  }
  await settle(page, 500);
}

export const shots = [
  {
    name: 'automations/list',
    path: '/settings/automations',
    clip: (p) => [p.locator('h1').last(), button(p, 'New automation'), row(p, 'Mark gift card mail as spam')],
    pad: 28,
    highlight: [
      { target: (p) => button(p, 'New automation'), label: 1, badge: 'l' },
      { target: (p) => [p.getByText('Route billing mail to the Billing team', { exact: true }), p.getByText(/^Ticket created · 1 condition · 2 actions$/).first()], label: 2, inset: 6 },
      { target: (p) => [p.getByText('412 runs', { exact: true }), row(p, 'Route billing mail to the Billing team').locator('button[role="switch"]')], label: 3, inset: 6, badge: 'tr' },
      { target: (p) => row(p, 'Mark gift card mail as spam').getByText('Paused', { exact: true }).first(), label: 4, inset: { x: 6, y: 4 }, badge: 'tr' },
    ],
  },
  {
    name: 'automations/editor',
    path: '/settings/automations',
    viewport: { width: 1440, height: 1000 },
    run: (page) => openAutomation(page, 'Escalate SSO outages'),
    clip: (p) => [p.getByText('Ran 37 times').first(), step(p, 'Then')],
    pad: 28,
    highlight: [
      { target: (p) => p.getByText('A ticket is created', { exact: true }).first().locator('xpath=ancestor::button[1]'), label: 1, inset: 4, badge: 'r' },
      { target: (p) => step(p, 'If all of these match'), label: 2, inset: 2 },
      { target: (p) => step(p, 'Then'), label: 3, inset: 2 },
      { target: (p) => p.locator('main button[role="switch"]').first().locator('xpath=..'), label: 4, inset: 3, badge: 'tr' },
    ],
  },
  {
    name: 'automations/condition-fields',
    path: '/settings/automations',
    viewport: { width: 1440, height: 1100 },
    run: async (page) => {
      await openAutomation(page, 'Escalate SSO outages');
      await step(page, 'If all of these match').getByText('Subject', { exact: true }).first().click();
      await settle(page);
    },
    keepFocus: true,
    clip: (p) => [step(p, 'If all of these match'), popover(p)],
    pad: 20,
    highlight: [
      { target: (p) => popover(p).locator('[role="option"]').filter({ hasText: /^\s*(Team|Brand|Chat widget|Address written to)\s*$/ }).all(), label: 1, inset: 2, badge: 'r' },
    ],
  },
  {
    name: 'automations/action-types',
    path: '/settings/automations',
    viewport: { width: 1440, height: 1200 },
    run: async (page) => {
      await openAutomation(page, 'Escalate SSO outages');
      await step(page, 'Then').getByText('Set priority to', { exact: true }).first().click();
      await settle(page);
    },
    keepFocus: true,
    clip: (p) => [step(p, 'Then'), popover(p)],
    pad: 20,
    highlight: [
      { target: (p) => popover(p).locator('[role="option"]').filter({ hasText: /^\s*Set reply address\s*$/ }).first(), label: 1, inset: 2, badge: 'r' },
    ],
  },
];
