// Sending services that relay over SMTP: picking one fills the server, port, security
// and, where the service fixes it, the username. The mailbox's domain is verified with
// the service, which then sends as the address.

import { useTranslation } from "react-i18next";
import type { TlsMode } from "@/lib/api/types";
import { cn } from "@/lib/cn";

export type SmtpPreset = {
  id: "resend" | "postmark" | "sendgrid" | "mailgun" | "brevo" | "ses";
  name: string;
  host: string;
  port: number;
  tls: Exclude<TlsMode, "none">;
  /** The username the service always uses; null when it is the account's own. */
  username: string | null;
};

export const SMTP_PRESETS: readonly SmtpPreset[] = [
  { id: "resend", name: "Resend", host: "smtp.resend.com", port: 465, tls: "tls", username: "resend" },
  { id: "postmark", name: "Postmark", host: "smtp.postmarkapp.com", port: 587, tls: "starttls", username: null },
  { id: "sendgrid", name: "SendGrid", host: "smtp.sendgrid.net", port: 587, tls: "starttls", username: "apikey" },
  { id: "mailgun", name: "Mailgun", host: "smtp.mailgun.org", port: 587, tls: "starttls", username: null },
  { id: "brevo", name: "Brevo", host: "smtp-relay.brevo.com", port: 587, tls: "starttls", username: null },
  { id: "ses", name: "Amazon SES", host: "email-smtp.us-east-1.amazonaws.com", port: 587, tls: "starttls", username: null },
];

/** The preset a server matches, by host (Mailgun's EU host and any SES region count). */
export function presetFor(host: string | undefined): SmtpPreset | null {
  const h = (host ?? "").trim().toLowerCase();
  if (!h) return null;
  if (h === "smtp.eu.mailgun.org") return SMTP_PRESETS.find((p) => p.id === "mailgun")!;
  if (/^email-smtp\.[a-z0-9-]+\.amazonaws\.com$/.test(h)) return SMTP_PRESETS.find((p) => p.id === "ses")!;
  return SMTP_PRESETS.find((p) => p.host === h) ?? null;
}

/** "Using a sending service?" and one chip per service; the picked one says what goes in the fields. */
export function SmtpPresets({ host, onPick }: { host: string | undefined; onPick: (p: SmtpPreset) => void }) {
  const { t } = useTranslation("settingsChannels");
  const current = presetFor(host);
  return (
    <div className="flex flex-col gap-2">
      <div className="flex flex-wrap items-center gap-1.5">
        <span className="mr-1 text-xs text-fg-3">{t("smtpPresets.label")}</span>
        {SMTP_PRESETS.map((p) => (
          <button
            key={p.id}
            type="button"
            aria-pressed={current?.id === p.id}
            onClick={() => onPick(p)}
            className={cn(
              "h-6 rounded-md border px-2 text-xs font-medium outline-none transition-colors duration-150 focus-visible:shadow-[0_0_0_3px_var(--focus)] active:scale-[0.97]",
              current?.id === p.id ? "border-inverse bg-inverse text-inverse-fg" : "border-line bg-surface text-fg-2 hover:border-line-strong hover:text-fg",
            )}
          >
            {p.name}
          </button>
        ))}
      </div>
      {current && (
        <p key={current.id} className="animate-fade-in text-xs leading-[18px] text-fg-3 motion-reduce:animate-none">
          {t(`smtpPresets.hint.${current.id}`)}
        </p>
      )}
    </div>
  );
}
