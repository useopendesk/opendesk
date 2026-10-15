import type { APIRoute, GetStaticPaths } from 'astro';
import { getCollection, type CollectionEntry } from 'astro:content';
import { toMarkdown } from '../lib/markdown';

// Each docs page as plain Markdown, for copying into an editor or an AI assistant.
export const getStaticPaths: GetStaticPaths = async () => {
  const docs = await getCollection('docs');
  return docs
    .filter((entry) => entry.id !== 'index' && entry.id !== '404')
    .map((entry) => ({ params: { slug: entry.id }, props: { entry } }));
};

export const GET: APIRoute<{ entry: CollectionEntry<'docs'> }> = ({ props, site }) => {
  const { title, description } = props.entry.data;
  const body = toMarkdown(props.entry.body ?? '', site);
  const head = `# ${title}\n\n${description ? description + '\n\n' : ''}`;
  return new Response(head + body + '\n', {
    headers: { 'Content-Type': 'text/markdown; charset=utf-8' },
  });
};
