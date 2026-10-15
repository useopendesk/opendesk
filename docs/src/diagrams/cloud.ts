import type { DiagramSpec } from './types';

// deploy/terraform and docs/src/content/docs/cloud/architecture.md.
export const cloud: DiagramSpec = {
  alt: 'The hosted service. Browsers and apps load the web app from Cloudflare Pages and reach the API and the realtime gateway through Cloudflare and an AWS load balancer. On AWS, ECS Fargate runs the api, gateway and worker services; workers publish events to a three-node NATS cluster, which the gateways subscribe to. The api uses Neon through the pooled host, the worker through the direct host. Cloudflare Email Routing and an Email Worker receive forwarded mail and post it to the API. S3, SES and Secrets Manager serve the api and workers.',
  caption: 'The same code as a self-host, split into roles: api, worker and gateway scale on their own.',
  width: 720,
  height: 488,
  groups: [
    { x: 160, y: 16, w: 160, h: 376, label: 'Cloudflare', tone: 'amber' },
    { x: 344, y: 16, w: 368, h: 376, label: 'AWS', tone: 'blue' },
  ],
  boxes: [
    { id: 'clients', x: 16, y: 88, w: 120, h: 64, title: 'Browsers', sub: 'and the apps' },
    { id: 'senders', x: 16, y: 304, w: 120, h: 64, title: 'Mail senders', sub: 'to <token>@in…' },
    { id: 'pages', x: 176, y: 44, w: 128, h: 64, title: 'Pages', sub: 'app.opendesk.sh,\nstatic' },
    { id: 'edge', x: 176, y: 132, w: 128, h: 64, title: 'DNS and edge', sub: 'api. and gateway.' },
    { id: 'email', x: 176, y: 296, w: 128, h: 80, title: 'Email Routing', sub: 'in.opendesk.sh,\nthen the Email\nWorker' },
    { id: 'alb', x: 360, y: 132, w: 96, h: 64, title: 'ALB', sub: 'TLS from ACM' },
    { id: 'api', x: 488, y: 44, w: 200, h: 56, title: 'api', mono: true, sub: 'ECS Fargate, the REST API' },
    { id: 'gateway', x: 488, y: 136, w: 200, h: 56, title: 'gateway', mono: true, sub: 'ECS Fargate, Phoenix' },
    { id: 'nats', x: 488, y: 224, w: 200, h: 52, title: 'NATS', sub: 'three-node JetStream cluster' },
    { id: 'worker', x: 488, y: 308, w: 200, h: 56, title: 'worker', mono: true, sub: 'ECS Fargate, relay and jobs' },
    { id: 's3', x: 360, y: 232, w: 96, h: 36, title: 'S3', sub: 'attachments', small: true },
    { id: 'ses', x: 360, y: 276, w: 96, h: 36, title: 'SES', sub: 'platform email', small: true },
    { id: 'secrets', x: 360, y: 320, w: 96, h: 36, title: 'Secrets Manager', sub: 'and KMS', small: true },
    { id: 'neon', x: 488, y: 416, w: 200, h: 60, title: 'Neon Postgres', sub: 'pooled and direct hosts', shape: 'db' },
  ],
  edges: [
    { from: 'clients', to: 'pages', fromAt: 0.3 },
    { from: 'clients', to: 'edge', fromAt: 0.7 },
    { from: 'edge', to: 'alb' },
    { from: 'alb', to: 'api', fromSide: 'r', fromAt: 0.25, toSide: 'l' },
    { from: 'alb', to: 'gateway', fromSide: 'r', fromAt: 0.6, toSide: 'l', toAt: 0.5 },
    { from: 'worker', to: 'nats', label: 'publish', labelAt: [596, 296] },
    { from: 'nats', to: 'gateway', label: 'subscribe', labelAt: [596, 212] },
    { from: 'senders', to: 'email' },
    { from: 'email', to: 'edge', fromSide: 't', toSide: 'b', label: 'posts mail', labelAt: [248, 252], labelAnchor: 'start' },
    { from: 'api', to: 'neon', fromSide: 'r', toSide: 'r', via: [[704, 72], [704, 446]], label: 'pooled', labelAt: [698, 410], labelAnchor: 'end' },
    { from: 'worker', to: 'neon', label: 'direct', labelAt: [596, 410] },
  ],
};
