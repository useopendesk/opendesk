import type { DiagramSpec } from './types';

// docs/src/content/docs/api/authentication.md, contracts/tokens.md and AGENTS.md
// "Access control".
export const apiAuth: DiagramSpec = {
  alt: 'How the API decides what a request may do. The bearer credential is either an API key, stored hashed and bound to one workspace, or a user access token, a 15-minute JWT with the purpose access, sent with the OD-Workspace header. An API key is limited by its scopes; a user by their role in the workspace. Either way, every row is read and written in the workspace of the key or the member, never one named in the body. Missing or invalid credentials answer 401, a missing scope or role 403, and a ticket the caller may not see 404.',
  caption: 'Two kinds of credential, one workspace. The request body never chooses the workspace.',
  width: 720,
  height: 252,
  boxes: [
    { id: 'request', x: 16, y: 88, w: 124, h: 72, title: 'Request', sub: 'Authorization:\nBearer …' },
    { id: 'key', x: 180, y: 24, w: 176, h: 72, title: 'API key', sub: 'od_live_…, stored hashed,\nbound to one workspace' },
    { id: 'token', x: 180, y: 152, w: 176, h: 72, title: 'Access token', sub: 'a 15-minute JWT, purpose\naccess, plus OD-Workspace' },
    { id: 'scopes', x: 392, y: 24, w: 160, h: 72, title: 'Scopes', sub: 'each route needs one,\nsuch as tickets:read' },
    { id: 'role', x: 392, y: 152, w: 160, h: 72, title: 'Role', sub: 'owner, admin, agent\nor viewer' },
    { id: 'workspace', x: 584, y: 72, w: 124, h: 104, title: 'Workspace', sub: 'every row read\nand written in the\nkey\'s or member\'s\nworkspace', tone: 'ink' },
  ],
  edges: [
    { from: 'request', to: 'key', fromAt: 0.3 },
    { from: 'request', to: 'token', fromAt: 0.7 },
    { from: 'key', to: 'scopes' },
    { from: 'token', to: 'role' },
    { from: 'scopes', to: 'workspace', toAt: 0.3 },
    { from: 'role', to: 'workspace', toAt: 0.7 },
  ],
  notes: [
    { x: 268, y: 128, anchor: 'middle', text: '401 unauthenticated', mono: true, tone: 'red' },
    { x: 472, y: 128, anchor: 'middle', text: '403 forbidden', mono: true, tone: 'red' },
    { x: 646, y: 200, anchor: 'middle', text: 'a hidden ticket: 404', mono: true, tone: 'red' },
  ],
};
