// A docs page's MDX source as plain Markdown, for "Copy Markdown" and the .md copies:
// imports dropped, and the docs components written out as Markdown (a screenshot as
// an image link with its alt text, steps as numbered titles, callouts as quotes, a
// diagram as its description).
import { diagrams } from '../diagrams';

const attrs = (s: string): Record<string, string> =>
  Object.fromEntries([...s.matchAll(/([\w-]+)="([^"]*)"/g)].map((m) => [m[1], m[2]]));

const calloutTitles: Record<string, string> = { tip: 'Good to know', note: 'Note', warning: 'Warning' };

export function toMarkdown(body: string, site: URL | undefined): string {
  // Code blocks stay as written (an `import` in a sample is part of the sample).
  const parts = body.split(/(^(?:```|~~~)[^\n]*\n[\s\S]*?^(?:```|~~~)[ \t]*$)/m);
  // Step numbers run on across a code block inside a step.
  const step = { n: 0 };
  const md = parts.map((part, i) => (i % 2 ? part : prose(part, site, step))).join('');
  return md.replace(/\n{3,}/g, '\n\n').trim();
}

function prose(text: string, site: URL | undefined, step: { n: number }): string {
  const abs = (src: string) => (site ? new URL(src, site).toString() : src);
  let md = text.replace(/^import .+ from .+;?\n/gm, '');

  md = md.replace(/<Screenshot\b([^>]*?)\/>/g, (_, a: string) => {
    const { src = '', alt = '', caption } = attrs(a);
    const file = src.replace(/(\.dark)?\.webp$/, '') + '.webp';
    return `![${alt}](${abs(file)})${caption ? `\n\n*${caption}*` : ''}`;
  });

  md = md.replace(/<Diagram\b([^>]*?)\/>/g, (_, a: string) => {
    const { name = '', caption } = attrs(a);
    const spec = (diagrams as Record<string, { alt: string; caption?: string }>)[name];
    if (!spec) return '';
    const note = caption ?? spec.caption;
    return `*Diagram: ${spec.alt}*${note ? `\n\n*${note}*` : ''}`;
  });

  md = md.replace(/<Callout\b([^>]*)>([\s\S]*?)<\/Callout>/g, (_, a: string, inner: string) => {
    const { type = 'tip', title } = attrs(a);
    const lines = [`**${title ?? calloutTitles[type] ?? 'Note'}**`, '', ...inner.trim().split('\n')];
    return lines.map((l) => (l.trim() ? `> ${l.trimStart()}` : '>')).join('\n');
  });

  md = md.replace(/<Steps>|<\/Steps>|<Step\b([^>]*)>|<\/Step>/g, (tag: string, a?: string) => {
    if (tag === '<Steps>') step.n = 0;
    return a !== undefined ? `**${++step.n}. ${attrs(a).title ?? ''}**` : '';
  });

  // Cards: a link with its line of text.
  md = md.replace(/<GuideCard\b([^>]*?)\/>/g, (_, a: string) => {
    const { title = '', href = '', description } = attrs(a);
    return `- [${title}](${href})${description ? `: ${description}` : ''}`;
  });
  md = md.replace(/<StartPath\b([^>]*)>([\s\S]*?)<\/StartPath>/g, (_, a: string, inner: string) => {
    const { title = '', href = '', cta } = attrs(a);
    return `- [${title}](${href}): ${inner.trim()}${cta ? ` (${cta})` : ''}`;
  });

  return md.replace(/<\/?(Legend|Fold)>|<Cards\b[^>]*>|<\/Cards>/g, '');
}
