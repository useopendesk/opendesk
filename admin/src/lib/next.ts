/**
 * A `next` path inside the panel, from a query string: one leading slash, never
 * another origin ("//evil", "/\\evil"), never the sign-in page itself.
 */
export function safeNext(v: unknown): string | undefined {
  if (typeof v !== "string" || !v.startsWith("/") || v.startsWith("//") || v.includes("\\")) return undefined;
  for (let i = 0; i < v.length; i++) if (v.charCodeAt(i) < 0x20 || v.charCodeAt(i) === 0x7f) return undefined;
  if (v === "/login" || v.startsWith("/login?") || v.startsWith("/login/")) return undefined;
  return v;
}

/** Splits a safe `next` into what router.navigate takes. */
export function nextTarget(next: string | undefined): { to: string; search: Record<string, string> } {
  const u = new URL(next ?? "/", "http://panel.invalid");
  return { to: u.pathname, search: Object.fromEntries(u.searchParams) };
}
