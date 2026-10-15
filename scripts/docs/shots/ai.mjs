// Shots for docs/src/content/docs/guides/ai.mdx ("AI features"): workspace AI settings,
// writing with AI, the AI agent for chat and email, your own provider key and costs.
import { around, block, button, labelled, settle, text } from '../helpers.mjs';

const TALL = { width: 1440, height: 1400 };

/** The box of the nearest bordered, rounded ancestor of `locator`: a settings card. */
export function framed(locator, minWidth = 200) {
  return locator.evaluate((el, minWidth) => {
    for (let e = el; e; e = e.parentElement) {
      const cs = getComputedStyle(e);
      if (parseFloat(cs.borderTopWidth) > 0 && parseFloat(cs.borderTopLeftRadius) > 0 && e.getBoundingClientRect().width >= minWidth) {
        const r = e.getBoundingClientRect();
        return { x: r.x, y: r.y, width: r.width, height: r.height };
      }
    }
    throw new Error('no framed ancestor');
  }, minWidth);
}

/** A settings row around its label: the nearest ancestor that also holds the row's control. */
export function row(p, label) {
  return p
    .getByText(label, { exact: true })
    .first()
    .evaluate((el) => {
      const control = 'button, input, textarea, select, [role="switch"], [role="radio"], [contenteditable="true"]';
      for (let e = el.parentElement; e; e = e.parentElement) {
        if (e.querySelector(control)) {
          const r = e.getBoundingClientRect();
          return { x: r.x, y: r.y, width: r.width, height: r.height };
        }
      }
      throw new Error(`no row for ${el.textContent}`);
    });
}
/** A page's own title, not the settings menu link of the same name. */
export const title = (p, t) => p.locator('h1, h2').filter({ hasText: new RegExp(`^\\s*${t}\\s*$`) }).last();

const card = (p, t) => framed(p.getByText(t, { exact: true }).first());
// Scrolls a section to the top of the page, so a tall crop fits under it.
const scrollTo = async (page, t) => {
  await page.getByText(t, { exact: true }).first().evaluate((el) => el.scrollIntoView({ block: 'start' }));
  await settle(page, 250);
};

async function openSso(page) {
  await page.getByText('SSO login loops back to sign-in page').first().click();
  await settle(page, 500);
}

export const shots = [
  // Settings, AI: the switches at the top of the page.
  {
    name: 'ai/settings',
    path: '/settings/ai',
    viewport: { width: 1440, height: 1000 },
    highlight: [
      { target: (p) => card(p, 'Use AI in this workspace'), label: 1, inset: 4 },
      { target: (p) => card(p, 'Let teammates ask Ollie'), label: 2, inset: 4 },
      { target: (p) => card(p, 'Triage new tickets'), label: 3, inset: 4 },
      { target: (p) => p.getByText('about $0.021').first(), label: 4, inset: { x: 5, y: 4 }, badge: 'br' },
    ],
  },
  {
    name: 'ai/open-settings',
    path: '/settings/ai',
    dim: true,
    clip: () => [{ x: 0, y: 0, width: 1, height: 1 }, { x: 860, y: 899, width: 1, height: 1 }],
    pad: 0,
    highlight: [
      { target: (p) => p.getByRole('link', { name: 'Settings', exact: true }).last(), label: 1, badge: 'r' },
      { target: (p) => p.locator('a[href="/settings/ai"]').first(), label: 2, badge: 'r' },
    ],
  },
  {
    name: 'ai/writing',
    path: '/settings/ai',
    viewport: TALL,
    run: (page) => scrollTo(page, 'Writing'),
    clip: (p) => [text(p, 'Writing'), card(p, 'Tone')],
    pad: 22,
    highlight: [
      { target: (p) => row(p, 'Tone'), label: 1, inset: { x: 10, y: -6 } },
      { target: (p) => row(p, 'Reply language'), label: 2, inset: { x: 10, y: -6 } },
      { target: (p) => row(p, 'Instructions'), label: 3, inset: { x: 10, y: -6 } },
    ],
  },
  {
    name: 'ai/own-key',
    path: '/settings/ai',
    viewport: TALL,
    run: async (page) => {
      await scrollTo(page, 'Provider');
      await page.getByText('Your own key', { exact: true }).first().click();
      await settle(page, 400);
    },
    clip: (p) => [text(p, 'Provider'), card(p, 'Runs on')],
    pad: 22,
    highlight: [{ target: (p) => p.getByText('Your own key', { exact: true }).first(), label: 1, inset: { x: 6, y: 4 } }],
  },
  {
    name: 'ai/models',
    path: '/settings/ai',
    viewport: TALL,
    run: (page) => scrollTo(page, 'Model'),
    clip: (p) => [text(p, 'Model'), card(p, 'Main model')],
    pad: 22,
    highlight: [
      { target: (p) => p.locator('button').filter({ hasText: 'Claude Sonnet 5.5' }).first(), label: 1, inset: 3, badge: 'r' },
      { target: (p) => p.locator('button').filter({ hasText: 'Claude Haiku 5.5' }).first(), label: 2, inset: 3, badge: 'r' },
    ],
  },
  {
    name: 'ai/costs',
    path: '/settings/ai',
    viewport: TALL,
    run: (page) => scrollTo(page, 'What AI costs'),
    clip: (p) => [text(p, 'What AI costs'), card(p, 'Summarize a ticket')],
    pad: 22,
  },

  // Writing with AI in the reply box.
  {
    name: 'ai/composer',
    path: '/inbox/open',
    viewport: { width: 1440, height: 1000 },
    run: async (page) => {
      await openSso(page);
      await page.locator('[contenteditable="true"]').first().click();
      await page.keyboard.type('hi dana, got the fingerprint, will add it today and let u know when its live');
      await labelled(page, 'Write with AI').click();
      await settle(page);
    },
    clip: (p) => [p.getByRole('menu'), around(p, 'Replying as', 'Send')],
    pad: 20,
    highlight: [
      { target: (p) => labelled(p, 'Write with AI'), label: 1, badge: 'r' },
      { target: (p) => [p.getByRole('menuitem', { name: /Draft a reply/ }), p.getByRole('menuitem', { name: /Draft with/ })], label: 2, inset: 2, badge: 'l' },
      { target: (p) => [p.getByRole('menuitem', { name: /Friendlier/ }), p.getByRole('menuitem', { name: /Simpler/ })], label: 3, inset: 2, badge: 'l' },
    ],
  },

  // The AI agent.
  {
    name: 'ai/agent-overview',
    path: '/settings/ai/agent',
    viewport: { width: 1440, height: 1100 },
    highlight: [
      { target: (p) => p.locator('a[href="/settings/ai/agent"]').first(), label: 1, inset: 3, badge: 'r' },
      { target: (p) => [p.locator('a[href="/settings/ai/behaviour"]').first(), p.locator('a[href="/settings/ai/tests"]').first()], label: 2, inset: 3, badge: 'r' },
      { target: (p) => framed(text(p, 'Your agent is set up')), label: 3, inset: 3 },
      { target: (p) => framed(text(p, 'Conversations'), 600), label: 4, inset: 3 },
      { target: (p) => [text(p, 'Needs review'), framed(text(p, 'My login link keeps expiring before I can click it'), 600)], label: 5, inset: 4 },
    ],
  },
  {
    name: 'ai/agent-behaviour',
    path: '/settings/ai/behaviour',
    viewport: TALL,
    clip: (p) => [title(p, 'Behaviour'), framed(p.getByText('Margin Concierge', { exact: true }).first())],
    pad: 22,
    highlight: [
      { target: (p) => card(p, 'Let the agent answer chats'), label: 1, inset: 3 },
      { target: (p) => framed(p.getByText('Margin Concierge', { exact: true }).first()), label: 2, inset: 3 },
      { target: (p) => button(p, 'Manage bots'), label: 3, badge: 'l' },
      { target: (p) => [button(p, 'Run tests'), button(p, 'Open playground')], label: 4, inset: 3 },
    ],
  },
  {
    name: 'ai/agent-guardrails',
    path: '/settings/ai/behaviour',
    viewport: { width: 1440, height: 1800 },
    run: (page) => scrollTo(page, 'Guardrails'),
    clip: (p) => [text(p, 'Model and limits'), card(p, 'Topics to decline')],
    pad: 22,
    highlight: [
      { target: (p) => card(p, 'Agent model'), label: 1, inset: 3 },
      { target: (p) => [row(p, 'Topics to decline'), text(p, '3 / 20', false)], label: 2, inset: { x: 10, y: -6 } },
      { target: (p) => [row(p, 'Monthly spending cap'), button(p, '$100')], label: 3, inset: { x: 10, y: -6 } },
      { target: (p) => row(p, 'Ask for an email before handing off'), label: 4, inset: { x: 10, y: -6 } },
    ],
  },
  {
    name: 'ai/agent-knowledge',
    path: '/settings/ai/knowledge',
    run: async (page) => {
      await button(page, 'Add knowledge').click();
      await settle(page);
    },
    clip: (p) => [p.locator('h1, h2').filter({ hasText: /^Knowledge$/ }).last(), p.locator('[role="menu"]'), text(p, 'Old help center')],
    pad: 22,
    highlight: [
      { target: (p) => button(p, 'Add knowledge'), label: 1, badge: 'l' },
      { target: (p) => p.locator('[role="menu"]'), label: 2, inset: 3, badge: 'l' },
      // The source's row: the nearest ancestor of its name that also holds its switch.
      {
        target: (p) =>
          text(p, 'Margin docs').evaluate((el) => {
            for (let e = el.parentElement; e; e = e.parentElement)
              if (e.querySelector('[role="switch"]')) {
                const r = e.getBoundingClientRect();
                return { x: r.x, y: r.y, width: r.width, height: r.height };
              }
            throw new Error('no row');
          }),
        label: 3,
        inset: 2,
        badge: 'l',
      },
    ],
  },
  {
    name: 'ai/agent-playground',
    path: '/settings/ai/playground',
    run: async (page) => {
      const box = page.getByPlaceholder('Write as a visitor');
      await box.fill('Does the Business plan include SSO?');
      await box.press('Enter');
      await page.waitForTimeout(7000);
    },
    clip: (p) => [title(p, 'Playground'), around(p, 'Try different settings', 'Answer as'), around(p, 'Visitor', 'Verified identity'), p.getByPlaceholder('Write as a visitor')],
    pad: 22,
    highlight: [
      { target: (p) => around(p, 'Does the Business plan include SSO?').last(), label: 1, inset: 3, badge: 'l' },
      { target: (p) => [p.getByText('Set up SAML single sign-on').first(), p.getByText('Change your plan', { exact: true }).last()], label: 2, inset: 6, badge: 'bl' },
      { target: (p) => around(p, 'Try different settings', 'Answer as', 'Agent model'), label: 3, inset: 3 },
      { target: (p) => around(p, 'Visitor', 'Verified identity'), label: 4, inset: 3 },
    ],
  },
  {
    name: 'ai/agent-tools',
    path: '/settings/ai/tools',
    clip: (p) => [p.getByRole('heading', { name: 'Tools', exact: true }).last(), button(p, 'Connect a server'), around(p, 'Shop', 'Linear', 'Needs sign-in')],
    pad: 22,
    highlight: [
      { target: (p) => button(p, 'Connect a server'), label: 1, badge: 'l' },
      { target: (p) => around(p, 'Shop', 'margin-shop', 'Connected').last(), label: 2, inset: 0, badge: 'l' },
      { target: (p) => [p.getByText('Needs sign-in').first(), button(p, 'Connect')], label: 3, inset: 5 },
    ],
  },

  // Email answers.
  {
    name: 'ai/mailbox-email',
    path: '/settings/mailboxes',
    viewport: { width: 1440, height: 2600 },
    run: async (page) => {
      await page.getByText('support@margin.app', { exact: true }).first().click();
      await settle(page, 600);
      await page.getByText('answers emails to this mailbox').first().scrollIntoViewIfNeeded();
      await settle(page, 300);
    },
    clip: (p) => [p.locator('h2, h3').filter({ hasText: /^\s*(Ollie|Pip|Margin Concierge)\s*$/ }).last(), framed(p.getByText('Only outside business hours', { exact: true }).first())],
    pad: 22,
    highlight: [
      { target: (p) => p.getByRole('switch', { name: /answers emails to this mailbox/ }), label: 1, inset: 5, badge: 'l' },
      { target: (p) => p.locator('#mailbox-bot'), label: 2, inset: 4, badge: 'l' },
      { target: (p) => [text(p, 'Draft for review'), text(p, 'Send automatically')], label: 3, inset: 5, badge: 'l' },
      { target: (p) => p.getByRole('switch', { name: 'Only outside business hours' }), label: 4, inset: 5, badge: 'l' },
    ],
  },
  {
    name: 'ai/suggested-reply',
    path: '/inbox/unassigned',
    viewport: TALL,
    run: async (page) => {
      await page.getByText("Can't export notebook to PDF").first().click();
      await settle(page, 700);
    },
    clip: (p) => framed(p.getByText('Nothing is sent until you send it').first(), 400),
    pad: 20,
    highlight: [
      { target: (p) => p.getByText('Nothing is sent until you send it').first(), label: 1, inset: { x: 6, y: 4 }, badge: 'tr' },
      { target: (p) => [p.getByText('Based on', { exact: true }).last(), p.getByText('PDF export fails for large notebooks').last()], label: 2, inset: 4, badge: 'tr' },
      { target: (p) => [button(p, 'Send'), button(p, 'Edit')], label: 3, inset: 4, badge: 'bl' },
      { target: (p) => button(p, 'Discard'), label: 4, inset: 4, badge: 'bl' },
    ],
  },
  {
    name: 'ai/chat-answer',
    path: '/inbox/open',
    viewport: TALL,
    run: async (page) => {
      await page.getByText('Cancel order #4821').first().click();
      await settle(page, 700);
    },
    clip: (p) => [framed(p.getByText('cancel order #4821?', { exact: false }).first(), 400), text(p, 'handed this to the team', false)],
    pad: 20,
    highlight: [
      { target: (p) => framed(p.getByText('The visitor confirmed and it ran').first(), 300), label: 1, inset: 3, badge: 'tr' },
      { target: (p) => p.locator('button').filter({ hasText: 'Improve answer' }).last(), label: 2, inset: 3, badge: 'tl' },
      { target: (p) => text(p, 'handed this to the team', false).locator('xpath=..'), label: 3, inset: { x: 8, y: 5 }, badge: 'l' },
    ],
  },
];
