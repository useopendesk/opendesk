import type { DiagramSpec } from './types';

// AGENTS.md "System shape", deploy/caddy/Caddyfile and docker-compose.yml.
export const selfHost: DiagramSpec = {
  alt: 'A self-hosted install on one server and one domain. Browsers reach Caddy over HTTPS on port 443. Caddy sends /socket to the realtime gateway and everything else to opendesk. opendesk, which runs the API, the workers, the web app and an embedded NATS, publishes events over NATS to the realtime gateway and keeps its data in Postgres. Realtime, opendesk and Postgres sit on an internal network; only Caddy is published.',
  caption: 'Four containers behind one domain. Only Caddy is published; the rest talk on an internal network.',
  width: 720,
  height: 328,
  groups: [
    { x: 152, y: 16, w: 556, h: 298, label: 'desk.example.com, one server' },
    { x: 340, y: 48, w: 352, h: 252, label: 'internal network', dashed: true },
  ],
  boxes: [
    { id: 'browser', x: 16, y: 128, w: 112, h: 64, title: 'Browser', sub: 'web app, apps\nand chat widget' },
    { id: 'caddy', x: 176, y: 128, w: 124, h: 64, title: 'caddy', mono: true, sub: 'HTTPS, ports 80, 443' },
    { id: 'realtime', x: 356, y: 80, w: 150, h: 56, title: 'realtime', mono: true, sub: 'live events, presence' },
    { id: 'opendesk', x: 356, y: 176, w: 150, h: 108, title: 'opendesk', mono: true, sub: 'API, workers, web app', valign: 'top' },
    { id: 'nats', x: 372, y: 236, w: 118, h: 32, title: 'NATS, embedded', small: true, tone: 'soft' },
    { id: 'postgres', x: 556, y: 184, w: 120, h: 72, title: 'postgres', mono: true, sub: 'the only datastore', shape: 'db' },
  ],
  edges: [
    { from: 'browser', to: 'caddy', label: ':443', mono: true },
    { from: 'caddy', to: 'realtime', fromSide: 't', toSide: 'l', label: '/socket', mono: true },
    { from: 'caddy', to: 'opendesk', fromSide: 'b', toSide: 'l', toAt: 0.35, label: 'everything else' },
    { from: 'opendesk', to: 'realtime', fromSide: 't', toSide: 'b', label: 'events over NATS', labelAt: [439, 160], labelAnchor: 'start' },
    { from: 'opendesk', to: 'postgres', fromAt: 0.4, label: 'SQL', mono: true },
  ],
  notes: [{ x: 176, y: 242, text: 'the only published service' }],
};
