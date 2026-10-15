import type { DiagramSpec } from './types';

// contracts/events.md "Transport", contracts/realtime.md "Channels" and contracts/teams.md
// "Visibility".
export const eventFlow: DiagramSpec = {
  alt: 'How a change reaches everyone. A request writes the change and an event row in one Postgres transaction. The relay, on the worker role, reads events in seq order and publishes each to NATS on od.events.<wid>; it also queues signed webhook deliveries. The realtime gateway pushes each event to its topics: workspace:<wid> gets every event, or only a stand-in for an event about a private team\'s ticket; team:<wid>:<tid> gets that team\'s events in full; user and contact topics get notifications and the chat widget\'s events. Browsers apply events, track seq on the workspace channel and fetch any gap from GET /v1/events.',
  caption: 'One write, one event, delivered live and to webhooks. A private team\'s events go only to that team.',
  width: 720,
  height: 404,
  minWidth: 600,
  groups: [{ x: 156, y: 8, w: 236, h: 96, label: 'one Postgres transaction' }],
  boxes: [
    { id: 'request', x: 16, y: 28, w: 116, h: 64, title: 'A request', sub: 'from the app, a key,\na job or an email' },
    { id: 'change', x: 168, y: 34, w: 104, h: 52, title: 'the change', sub: 'tickets, messages…', small: true },
    { id: 'event', x: 280, y: 34, w: 100, h: 52, title: 'an event row', sub: 'seq 1042', small: true, tone: 'violet' },
    { id: 'relay', x: 416, y: 28, w: 120, h: 64, title: 'Relay', sub: 'worker role,\nin seq order' },
    { id: 'nats', x: 568, y: 28, w: 136, h: 64, title: 'od.events.<wid>', mono: true, sub: 'NATS subject' },
    { id: 'webhooks', x: 416, y: 124, w: 120, h: 56, title: 'Webhooks', sub: 'signed, retried' },
    { id: 'endpoint', x: 248, y: 124, w: 144, h: 56, title: 'Your endpoint', sub: 'a private team\'s events\nonly when granted' },
    { id: 'gateway', x: 568, y: 212, w: 136, h: 176, title: 'Realtime gateway', sub: 'checks the token,\nthe origin and the\nteams claim' },
    { id: 'ws', x: 312, y: 212, w: 224, h: 56, title: 'workspace:<wid>', mono: true, sub: 'every event; a stand-in for\nan event of a private team' },
    { id: 'team', x: 312, y: 276, w: 224, h: 56, title: 'team:<wid>:<tid>', mono: true, sub: 'that private team\'s events,\nin full, to its members', tone: 'violet' },
    { id: 'user', x: 312, y: 340, w: 224, h: 48, title: 'user:<uid>, contact:…', mono: true, sub: 'notifications; the chat widget' },
    {
      id: 'browsers',
      x: 16,
      y: 212,
      w: 264,
      h: 176,
      title: 'Browsers and apps',
      points: ['apply each event to what is on screen', 'track seq on the workspace channel', 'fetch a gap from GET /v1/events', 'apply team events when they arrive'],
    },
  ],
  edges: [
    { from: 'request', to: 'change' },
    { from: 'event', to: 'relay' },
    { from: 'relay', to: 'nats' },
    { from: 'relay', to: 'webhooks', label: 'queues', labelAt: [484, 112], labelAnchor: 'start' },
    { from: 'webhooks', to: 'endpoint' },
    { from: 'nats', to: 'gateway' },
    { from: 'gateway', to: 'ws', fromSide: 'l', fromAt: 28 / 176, toSide: 'r' },
    { from: 'gateway', to: 'team', fromSide: 'l', fromAt: 92 / 176, toSide: 'r' },
    { from: 'gateway', to: 'user', fromSide: 'l', fromAt: 152 / 176, toSide: 'r' },
    { from: 'ws', to: 'browsers', toSide: 'r', toAt: 28 / 176 },
    { from: 'team', to: 'browsers', toSide: 'r', toAt: 92 / 176 },
    { from: 'user', to: 'browsers', toSide: 'r', toAt: 152 / 176 },
  ],
};
