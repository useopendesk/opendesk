// Every docs diagram by name, for <Diagram name="..." /> (components/Diagram.astro) and
// for the Markdown copy of a page (lib/markdown.ts), which writes out its alt text.
import type { DiagramSpec } from './types';
import { apiAuth } from './api-auth';
import { cloud } from './cloud';
import { emailIntake, reply, routingOrder } from './email';
import { eventFlow } from './event-flow';
import { selfHost } from './self-host';

export const diagrams = {
  'self-host': selfHost,
  cloud,
  'event-flow': eventFlow,
  'email-intake': emailIntake,
  'routing-order': routingOrder,
  reply,
  'api-auth': apiAuth,
} satisfies Record<string, DiagramSpec>;

export type DiagramName = keyof typeof diagrams;
