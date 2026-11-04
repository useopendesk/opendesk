// Release versions of the app (1.3.0, 1.4.0-beta.2), compared the way semver orders them.

function parse(v: string) {
  const [core = "", pre] = v.replace(/^v/, "").split("+")[0]!.split(/-(.*)/s);
  return { core: core.split(".").map((n) => Number(n) || 0), pre: pre ? pre.split(".") : [] };
}

/** Whether version a comes after b. A pre-release comes before its release. */
export function newerVersion(a: string, b: string): boolean {
  const x = parse(a);
  const y = parse(b);
  for (let i = 0; i < 3; i++) {
    const d = (x.core[i] ?? 0) - (y.core[i] ?? 0);
    if (d) return d > 0;
  }
  if (!x.pre.length || !y.pre.length) return !x.pre.length && !!y.pre.length;
  for (let i = 0; i < Math.max(x.pre.length, y.pre.length); i++) {
    const p = x.pre[i];
    const q = y.pre[i];
    if (p === undefined) return false;
    if (q === undefined) return true;
    if (p === q) continue;
    const pn = /^\d+$/.test(p);
    const qn = /^\d+$/.test(q);
    if (pn && qn) return Number(p) > Number(q);
    if (pn !== qn) return !pn;
    return p > q;
  }
  return false;
}
