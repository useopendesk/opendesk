// Templates in the reply composer (contracts/templates.md): what a picked
// variable writes, and the one-line preview of the signature a reply carries.

import { tagFor } from "@/components/templates/catalog";

/**
 * What picking a variable writes into a reply: this ticket's value as plain text,
 * so the agent reads real words. The tag itself when there is no value yet (the
 * server fills it when the reply is created), when the value is HTML (it is
 * filled as HTML on send), or when the value reads like a tag (the reply is
 * filled once more on send, and a value is never read again as a template).
 */
export function variableText(key: string, value: string | undefined, html = false): string {
  if (!value?.trim() || html || /\{\{|\{%/.test(value)) return tagFor(key);
  return value;
}

/** Plain text of a signature on one line, or null when it is empty or longer than `max`. */
export function signatureText(html: string, max = 120): string | null {
  if (!html) return null;
  const spaced = html.replace(/<br\s*\/?>/gi, " ").replace(/<\/(p|div|li|tr|td|th|h[1-6]|table|blockquote)>/gi, " ");
  // An inert document: nothing in it runs or loads.
  const doc = new DOMParser().parseFromString(spaced, "text/html");
  doc.querySelectorAll("style, script, template").forEach((el) => el.remove());
  const text = (doc.body.textContent ?? "").replace(/\s+/g, " ").trim();
  return text && text.length <= max ? text : null;
}
