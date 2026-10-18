// Display formatting. The panel is English: numbers and money in en-US
// ("$1,234.56"), dates relative in lists with the absolute time on hover.

const LOCALE = "en-US";

const cache = new Map<string, Intl.NumberFormat | Intl.DateTimeFormat>();
function nf(opts: Intl.NumberFormatOptions = {}): Intl.NumberFormat {
  const key = `n|${JSON.stringify(opts)}`;
  let f = cache.get(key) as Intl.NumberFormat | undefined;
  if (!f) cache.set(key, (f = new Intl.NumberFormat(LOCALE, opts)));
  return f;
}
function dtf(opts: Intl.DateTimeFormatOptions): Intl.DateTimeFormat {
  const key = `d|${JSON.stringify(opts)}`;
  let f = cache.get(key) as Intl.DateTimeFormat | undefined;
  if (!f) cache.set(key, (f = new Intl.DateTimeFormat(LOCALE, opts)));
  return f;
}

/** Grouped number: 12345 -> "12,345". */
export function number(n: number | null | undefined, opts?: Intl.NumberFormatOptions): string {
  if (n === null || n === undefined || !Number.isFinite(n)) return "–";
  return nf(opts).format(n);
}

/** 1234 -> "1.2K". */
export function compact(n: number): string {
  return nf({ notation: "compact", maximumFractionDigits: 1 }).format(n);
}

/** 0.874 -> "87%". */
export function percent(ratio: number, digits = 0): string {
  if (!Number.isFinite(ratio)) return "–";
  return nf({ style: "percent", maximumFractionDigits: digits }).format(ratio);
}

/**
 * US dollars: "$1,234.56". An amount under a cent keeps up to four decimals
 * ("$0.0042", AI calls cost fractions of a cent), and a tiny non-zero amount
 * reads "<$0.0001".
 */
export function usd(n: number | null | undefined): string {
  if (n === null || n === undefined || !Number.isFinite(n)) return "–";
  const a = Math.abs(n);
  if (a === 0 || a >= 0.01) return nf({ style: "currency", currency: "USD", minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(n);
  if (a < 0.0001) return `${n < 0 ? "-" : ""}<$0.0001`;
  return nf({ style: "currency", currency: "USD", minimumFractionDigits: 2, maximumFractionDigits: 4 }).format(n);
}

function parse(iso: string | null | undefined): number | null {
  if (!iso) return null;
  const t = Date.parse(iso);
  return Number.isFinite(t) ? t : null;
}

/** "just now", "5 minutes ago", "in 3 hours", "2 days ago", "Mar 4, 2025" after a month. */
export function relative(iso: string | null | undefined, now = Date.now()): string {
  const t = parse(iso);
  if (t === null) return "";
  const s = Math.round((now - t) / 1000);
  const abs = Math.abs(s);
  const ago = (n: number, unit: string) => {
    const u = `${n} ${unit}${n === 1 ? "" : "s"}`;
    return s >= 0 ? `${u} ago` : `in ${u}`;
  };
  if (abs < 45) return s >= 0 ? "just now" : "in a moment";
  const m = Math.round(abs / 60);
  if (m < 60) return ago(Math.max(1, m), "minute");
  const h = Math.round(m / 60);
  if (h < 24) return ago(h, "hour");
  const d = Math.round(h / 24);
  if (d < 30) return ago(d, "day");
  return shortDate(iso, now);
}

/** "Oct 6", or "Oct 6, 2024" outside the current year. */
export function shortDate(iso: string | null | undefined, now = Date.now()): string {
  const t = parse(iso);
  if (t === null) return "";
  const d = new Date(t);
  const same = d.getFullYear() === new Date(now).getFullYear();
  return dtf(same ? { month: "short", day: "numeric" } : { month: "short", day: "numeric", year: "numeric" }).format(d);
}

/** The absolute time for a tooltip: "Sat, Oct 11, 2026, 2:05:09 PM". */
export function fullDate(iso: string | null | undefined): string {
  const t = parse(iso);
  if (t === null) return "";
  return dtf({ weekday: "short", month: "short", day: "numeric", year: "numeric", hour: "numeric", minute: "2-digit", second: "2-digit" }).format(new Date(t));
}

/** A UTC calendar day for chart labels: "2026-10-04" -> "Oct 4". */
export function dayLabel(ymd: string): string {
  const [y, m, d] = ymd.split("-").map(Number);
  if (!y || !m || !d) return ymd;
  return dtf({ month: "short", day: "numeric", timeZone: "UTC" }).format(new Date(Date.UTC(y, m - 1, d)));
}

/** Seconds as a short age: 42 -> "42s", 135 -> "2m 15s", 7200 -> "2h", 90000 -> "1d 1h". */
export function duration(seconds: number | null | undefined): string {
  if (seconds === null || seconds === undefined || !Number.isFinite(seconds)) return "–";
  const s = Math.max(0, Math.round(seconds));
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  if (m < 60) return s % 60 ? `${m}m ${s % 60}s` : `${m}m`;
  const h = Math.floor(m / 60);
  if (h < 24) return m % 60 ? `${h}h ${m % 60}m` : `${h}h`;
  const d = Math.floor(h / 24);
  return h % 24 ? `${d}d ${h % 24}h` : `${d}d`;
}

/** English plural: plural(3, "member") -> "3 members". */
export function plural(n: number, one: string, many = `${one}s`) {
  return `${number(n)} ${n === 1 ? one : many}`;
}

/** "workspace.limits_changed" -> "Workspace limits changed". */
export function humanize(code: string): string {
  const s = code.replace(/[._]+/g, " ").trim();
  return s ? s.charAt(0).toUpperCase() + s.slice(1) : s;
}
