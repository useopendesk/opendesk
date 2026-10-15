// A mailbox's trusted contact form senders (contracts/api.md "Inbound email"):
// exact addresses or "@domain" entries whose verified mail is from the visitor
// named in Reply-To.

import { useTranslation } from "react-i18next";
import { ChipEditor } from "@/components/agent/ChipEditor";

export const FORM_SENDERS_MAX = 50;
const emailRe = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
const domainRe = /^@([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$/;

/** Providers anyone can sign up to: a domain entry for one of them would trust every user there. */
const PUBLIC_MAIL = new Set([
  "gmail.com", "googlemail.com", "outlook.com", "hotmail.com", "live.com", "msn.com", "yahoo.com", "ymail.com", "icloud.com", "me.com", "mac.com",
  "aol.com", "proton.me", "protonmail.com", "pm.me", "gmx.com", "gmx.net", "mail.com", "yandex.com", "yandex.ru", "zoho.com", "fastmail.com",
]);

/** Lowercases an entry; null when it is neither an address nor "@domain". */
export function normalizeFormSender(v: string): string | null {
  const x = v.trim().toLowerCase();
  if (x.startsWith("@")) return domainRe.test(x) ? x : null;
  return emailRe.test(x) ? x : null;
}

export function ContactFormsField({ value, onChange, own }: { value: string[]; onChange: (v: string[]) => void; own: string[] }) {
  const { t } = useTranslation("settingsChannels");
  const mine = own.map((a) => a.trim().toLowerCase());
  return (
    <ChipEditor
      mono
      values={value}
      onChange={onChange}
      max={FORM_SENDERS_MAX}
      normalize={normalizeFormSender}
      validate={(v) => {
        if (v.startsWith("@") && PUBLIC_MAIL.has(v.slice(1))) return t("forms.publicDomain", { domain: v });
        if (mine.includes(v)) return t("forms.ownAddress");
        return null;
      }}
      placeholder="website@company.com"
      aria-label={t("forms.title")}
      empty={t("forms.empty")}
    />
  );
}
