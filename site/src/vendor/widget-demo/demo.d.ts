// Types for demo.js, the live chat widget built from web/widget/src/site.ts
// (`pnpm build:site-widget` in web/). Rebuild it when the widget changes.
export type PreviewScene = 'launcher' | 'home' | 'chat';
export type PreviewOptions = { config: Record<string, unknown>; scene: PreviewScene; dark?: boolean; autoplay?: boolean };
export type PreviewHandle = { update(opts: Partial<PreviewOptions>): void; destroy(): void };
export function mountPreview(host: HTMLElement, opts: PreviewOptions): PreviewHandle;
export function demoConfig(key: string, params?: URLSearchParams): Record<string, unknown>;
