// Mail hosts: display names, logos, how a password signs in, and the one-line
// connection label the mailbox list shows ("Gmail · App password"). The API is the
// source of truth for detection (GET /v1/mailboxes/detect); this mirrors its tables
// for mailboxes that are already connected.

import type { Mailbox, MailHost, PasswordAuth } from "@/lib/api/types";

export const MAIL_HOST_LABELS: Record<MailHost, string> = {
  google_workspace: "Google Workspace",
  gmail: "Gmail",
  microsoft365: "Microsoft 365",
  outlook: "Outlook.com",
  zoho: "Zoho Mail",
  yahoo: "Yahoo Mail",
  aol: "AOL Mail",
  icloud: "iCloud Mail",
  fastmail: "Fastmail",
  godaddy: "GoDaddy",
  namecheap: "Namecheap Private Email",
  ionos: "IONOS",
  hostinger: "Hostinger",
  ovh: "OVHcloud",
  migadu: "Migadu",
  purelymail: "Purelymail",
  rackspace: "Rackspace Email",
  yandex: "Yandex Mail",
  gmx: "GMX",
  proton: "Proton Mail",
  other: "Other provider",
};

const HOST_LOGOS: Partial<Record<MailHost, string>> = {
  aol: "aol.png",
  fastmail: "fastmail.png",
  gmx: "gmx.png",
  godaddy: "godaddy.png",
  hostinger: "hostinger.png",
  icloud: "icloud.png",
  ionos: "ionos.svg",
  migadu: "migadu.png",
  namecheap: "namecheap.png",
  ovh: "ovh.svg",
  proton: "proton.png",
  purelymail: "purelymail.png",
  rackspace: "rackspace.png",
  yahoo: "yahoo.png",
  yandex: "yandex.png",
  zoho: "zoho.png",
};

export function isMailHost(v: unknown): v is MailHost {
  return typeof v === "string" && v in MAIL_HOST_LABELS;
}

export function mailHostLabel(h: string | null | undefined): string {
  return isMailHost(h) ? MAIL_HOST_LABELS[h] : "";
}

/** The shipped logo for a host, if any. */
export function mailHostImage(h: string | null | undefined): string | null {
  return isMailHost(h) && HOST_LOGOS[h] ? `/logos/hosts/${HOST_LOGOS[h]}` : null;
}

export const isGoogleHost = (h: string | null | undefined) => h === "gmail" || h === "google_workspace";
export const isMicrosoftHost = (h: string | null | undefined) => h === "microsoft365" || h === "outlook";

/** How a password signs in on a host (mirrors mailhost.PasswordAuthFor). */
export function passwordAuthFor(h: string | null | undefined): PasswordAuth {
  switch (h) {
    case "google_workspace":
    case "gmail":
    case "yahoo":
    case "aol":
    case "icloud":
    case "fastmail":
    case "yandex":
      return "app_password";
    case "microsoft365":
    case "outlook":
      return "oauth_only";
    case "proton":
      return "unsupported";
  }
  return "password";
}

/** Google prints an app password as four groups of four letters. */
export function stripAppPassword(raw: string): string {
  return raw.replace(/\s+/g, "");
}
export function looksLikeGoogleAppPassword(raw: string): boolean {
  return /^[a-z]{16}$/i.test(stripAppPassword(raw));
}

export type ConnectionMethod = "signin" | "app_password" | "password" | "forwarding";

/** How a connected mailbox signs in, from its kind and host. */
export function connectionMethod(m: Pick<Mailbox, "inbound_kind" | "outbound_kind" | "config">): ConnectionMethod {
  if (m.inbound_kind === "google" || m.inbound_kind === "microsoft") return "signin";
  if (m.inbound_kind === "forwarding") return "forwarding";
  return passwordAuthFor(m.config.mail_host) === "app_password" ? "app_password" : "password";
}

/** The host a connected mailbox is on, when known. */
export function mailboxHost(m: Pick<Mailbox, "inbound_kind" | "config" | "email">): MailHost | null {
  if (isMailHost(m.config.mail_host)) return m.config.mail_host;
  const domain = m.email.split("@")[1]?.toLowerCase() ?? "";
  if (m.inbound_kind === "google") return domain === "gmail.com" || domain === "googlemail.com" ? "gmail" : "google_workspace";
  if (m.inbound_kind === "microsoft") return "microsoft365";
  return null;
}

/**
 * Whether to offer an unencrypted connection: only for a local or private-network host,
 * which a development instance allows. Public mail servers always need TLS or STARTTLS.
 */
export function offersNoTls(host: string): boolean {
  const h = host.trim().toLowerCase().replace(/^\[|\]$/g, "");
  if (!h) return false;
  if (h === "localhost" || h.endsWith(".localhost") || h.endsWith(".local") || h.endsWith(".internal") || !h.includes(".")) return true;
  if (h === "::1" || h.startsWith("fc") || h.startsWith("fd")) return h.includes(":");
  const ip = /^(\d+)\.(\d+)\.\d+\.\d+$/.exec(h);
  if (!ip) return false;
  const [a, b] = [Number(ip[1]), Number(ip[2])];
  return a === 127 || a === 10 || (a === 172 && b >= 16 && b <= 31) || (a === 192 && b === 168);
}
