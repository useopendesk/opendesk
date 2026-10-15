import type { DiagramSpec } from './types';

// contracts/api.md "Email" and contracts/teams.md "Routing" and "Reply address".
export const emailIntake: DiagramSpec = {
  alt: 'How email becomes a ticket. Mail arrives by forwarding (the Email Worker, the built-in SMTP receiver or your own pipeline), from Google, from Microsoft or over IMAP. Intake skips loops and automatic mail, checks the sender\'s DMARC result, threads a reply to its ticket, stores each message once and finds or adds the contact. A reply is added to its ticket and never re-routed. A new conversation is routed to a team and becomes a new ticket, assigned by the team\'s method.',
  caption: 'Every way in ends at the same intake, so threading, sender checks and routing work the same for every mailbox.',
  width: 720,
  height: 276,
  boxes: [
    { id: 'forwarding', x: 16, y: 16, w: 160, h: 64, title: 'Forwarding', sub: 'Email Worker, SMTP\nreceiver or a pipeline' },
    { id: 'google', x: 16, y: 92, w: 160, h: 48, title: 'Google', sub: 'Gmail and Workspace' },
    { id: 'microsoft', x: 16, y: 152, w: 160, h: 48, title: 'Microsoft', sub: 'Microsoft 365, Outlook' },
    { id: 'imap', x: 16, y: 212, w: 160, h: 48, title: 'IMAP', sub: 'any other provider' },
    {
      id: 'intake',
      x: 208,
      y: 16,
      w: 228,
      h: 244,
      title: 'Intake',
      tone: 'soft',
      valign: 'middle',
      points: ['skips loops and automatic mail', 'checks the sender\'s DMARC', 'threads a reply to its ticket', 'stores each message once', 'finds or adds the contact'],
    },
    { id: 'existing', x: 480, y: 16, w: 224, h: 64, title: 'Added to its ticket', sub: 'wherever it was sent;\na reply is never re-routed' },
    { id: 'routing', x: 480, y: 106, w: 224, h: 64, title: 'Routed to a team', sub: 'address team, rules,\nthen triage' },
    { id: 'ticket', x: 480, y: 204, w: 224, h: 56, title: 'New ticket', sub: 'assigned by the team\'s method', tone: 'ink' },
  ],
  edges: [
    { from: 'forwarding', to: 'intake', toAt: 32 / 244 },
    { from: 'google', to: 'intake', toAt: 100 / 244 },
    { from: 'microsoft', to: 'intake', toAt: 160 / 244 },
    { from: 'imap', to: 'intake', toAt: 220 / 244 },
    { from: 'intake', to: 'existing', fromAt: 32 / 244, label: 'a reply' },
    { from: 'intake', to: 'routing', fromAt: 122 / 244, label: 'new' },
    { from: 'routing', to: 'ticket' },
  ],
};

export const routingOrder: DiagramSpec = {
  alt: 'The order in which a new ticket gets its team. First the place it arrived: the address\'s default team, or a chat\'s topic or widget. Then routing rules, in order, where the first match sets the team. Then triage, only while the ticket has no team. Automations run last and may still assign another team.',
  width: 720,
  height: 116,
  boxes: [
    { id: 'arrived', x: 15, y: 24, w: 156, h: 72, n: 1, title: 'Where it arrived', sub: 'the address\'s team, a\nchat\'s topic or widget' },
    { id: 'rules', x: 193, y: 24, w: 156, h: 72, n: 2, title: 'Routing rules', sub: 'in order; the first\nmatch sets the team' },
    { id: 'triage', x: 371, y: 24, w: 156, h: 72, n: 3, title: 'Triage', sub: 'only while the ticket\nhas no team' },
    { id: 'automations', x: 549, y: 24, w: 156, h: 72, n: 4, title: 'Automations', sub: 'may still assign\nanother team' },
  ],
  edges: [
    { from: 'arrived', to: 'rules' },
    { from: 'rules', to: 'triage' },
    { from: 'triage', to: 'automations' },
  ],
};

export const reply: DiagramSpec = {
  alt: 'How a reply goes out. The agent sends a reply. opendesk picks the address: the one chosen for this reply, else the ticket\'s reply address, else the brand\'s address for the ticket\'s team. The mailbox sends it through SMTP, Gmail or Microsoft from your own address, retrying up to 8 times. The customer receives it from your address, in the same thread, with the signature.',
  width: 720,
  height: 150,
  boxes: [
    { id: 'agent', x: 10, y: 24, w: 160, h: 88, n: 1, title: 'Your reply', sub: 'written in the reply box,\nsent with Send' },
    { id: 'address', x: 190, y: 24, w: 160, h: 88, n: 2, title: 'The reply address', sub: 'the one picked for it,\nelse the ticket\'s, else\nthe team\'s in the brand' },
    { id: 'send', x: 370, y: 24, w: 160, h: 88, n: 3, title: 'The mailbox sends it', sub: 'SMTP, Gmail or Microsoft,\nretried up to 8 times' },
    { id: 'customer', x: 550, y: 24, w: 160, h: 88, n: 4, title: 'The customer', sub: 'From your address, in\nthe same thread, with\nthe signature' },
  ],
  edges: [
    { from: 'agent', to: 'address' },
    { from: 'address', to: 'send' },
    { from: 'send', to: 'customer' },
  ],
  notes: [{ x: 360, y: 136, anchor: 'middle', text: 'The message shows its delivery status: pending, sent, failed or bounced.' }],
};
