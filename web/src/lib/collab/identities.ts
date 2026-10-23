// The addresses a mail account sends from (contracts/collab.md "Addresses"): its own
// and its aliases (Gmail's send-as addresses, ones added by hand for Microsoft and IMAP).
// The composer picks one: a reply defaults to the address the answered message was
// written to, new mail to the account's default. The server checks the choice again.

import type { MailIdentity, MailMessage } from "../api/types";
import { formatAddress, normEmail } from "./recipients";

/** What the helpers need of an account (MailAccount or a conversation's MailThreadAccount). */
export type IdentityAccount = { email: string; display_name: string; signature_html: string; identities?: MailIdentity[]; default_from?: string };

/** Its own address, as an identity, for shapes without the list. */
function primaryOf(a: IdentityAccount): MailIdentity {
  return {
    id: null,
    email: normEmail(a.email),
    display_name: a.display_name,
    signature_html: a.signature_html,
    reply_to: "",
    primary: true,
    default: true,
    source: "account",
    verification: "accepted",
    can_send: true,
    signature_sync: "none",
    display_name_sync: "none",
    signature_synced_at: null,
  };
}

/** Every address, its own first. */
export function identitiesOf(a: IdentityAccount): MailIdentity[] {
  return a.identities?.length ? a.identities : [primaryOf(a)];
}

/** The addresses that can send (a Gmail address waiting to be confirmed cannot). */
export function sendableIdentities(a: IdentityAccount): MailIdentity[] {
  return identitiesOf(a).filter((i) => i.can_send);
}

/** Every address of the account, lower case: its own and its aliases. */
export function ownEmails(a: IdentityAccount): string[] {
  return identitiesOf(a).map((i) => normEmail(i.email));
}

/** One of the account's sendable addresses, if email is. */
export function findIdentity(a: IdentityAccount, email: string | null | undefined): MailIdentity | undefined {
  if (!email) return undefined;
  const e = normEmail(email);
  return sendableIdentities(a).find((i) => normEmail(i.email) === e);
}

/** The address new mail goes out from. */
export function defaultIdentity(a: IdentityAccount): MailIdentity {
  return findIdentity(a, a.default_from) ?? sendableIdentities(a).find((i) => i.default) ?? identitiesOf(a)[0];
}

/**
 * The address a reply to m goes out from: the one it was sent from or written to (the
 * server says which, own_address; else its From, To and Cc), else the default.
 */
export function replyIdentity(a: IdentityAccount, m?: MailMessage): MailIdentity {
  if (!m) return defaultIdentity(a);
  const hit = findIdentity(a, m.own_address) ?? [m.from, ...m.to, ...m.cc].map((x) => findIdentity(a, x.email)).find(Boolean);
  return hit ?? defaultIdentity(a);
}

/** Its From name: its own, else the account's. */
export function identityName(i: MailIdentity, a: IdentityAccount): string {
  return i.display_name || a.display_name;
}

/** "Name <email>", the way the From picker shows it. */
export function fromLabel(i: MailIdentity, a: IdentityAccount): string {
  return formatAddress({ name: identityName(i, a), email: i.email });
}
