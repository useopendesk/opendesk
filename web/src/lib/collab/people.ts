// The people of a conversation for its people panel (contracts/collab.md "People"):
// everyone on it but the addresses of your own accounts, newest first.

import type { MailAddress } from "../api/types";
import { ownEmails, type IdentityAccount } from "./identities";
import { normEmail } from "./recipients";

/** Outside people on the conversation, newest first, each address once. */
export function peopleOf(participants: MailAddress[], accounts: IdentityAccount[]): MailAddress[] {
  const own = new Set(accounts.flatMap(ownEmails));
  const seen = new Set<string>();
  const out: MailAddress[] = [];
  for (const p of participants) {
    const e = normEmail(p.email);
    if (!e || own.has(e) || seen.has(e)) continue;
    seen.add(e);
    out.push({ name: p.name, email: e });
  }
  return out;
}

/** What to call someone: their name, else the address. */
export const personName = (p: { name: string; email: string }) => p.name.trim() || p.email;

/** Their first name for short labels ("Files from Dana"), else the address. */
export const firstName = (p: { name: string; email: string }) => p.name.trim().split(/\s+/)[0] || p.email;
