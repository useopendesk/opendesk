// The marketing site's live demo (opendesk.sh): the real widget against the
// in-memory backend, mounted into the page like the dashboard preview.
// Built by `pnpm build:site-widget` into site/src/vendor/widget-demo/.
export { mountPreview } from "./preview";
export type { PreviewHandle, PreviewOptions, PreviewScene } from "./preview";
export { demoConfig } from "./mock";
