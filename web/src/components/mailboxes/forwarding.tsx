// Turning on forwarding at the provider: the private address to forward to, short
// steps for the mailbox's host, each with a link straight to the settings page it
// names, and the provider's own guide and ours for the details.

import { ExternalLink, Send, TriangleAlert } from "lucide-react";
import { useState } from "react";
import { Trans, useTranslation } from "react-i18next";
import { CopyField } from "@/components/ui/copy-button";
import { Segmented } from "@/components/ui/tabs";
import type { Mailbox, MailHost } from "@/lib/api/types";
import { mailboxHost, mailHostLabel } from "@/lib/mailHost";
import { ExternalButton } from "./connect";

const DOCS = "https://docs.opendesk.sh/guides/mailboxes/forwarding/";

type Guide = "gmail" | "workspace" | "microsoft" | "outlook" | "other";
const GUIDES: readonly Guide[] = ["gmail", "workspace", "microsoft", "outlook", "other"];

/** A link to the page a step names; the label is forwarding.open.<key>. */
type StepLink = {
  key: "gmail" | "googleAdmin" | "defender" | "exchange" | "outlookForwarding" | "outlookCom";
  url: (email: string) => string;
};

/** Opens Gmail's forwarding tab signed in as the mailbox's address, so it lands on the right account. */
const gmailForwarding: StepLink = {
  key: "gmail",
  url: (email) => `https://mail.google.com/mail/?authuser=${encodeURIComponent(email)}#settings/fwdandpop`,
};

/** Each guide: its steps (forwarding.steps.<guide>.s1..sN), the page each step opens, the provider's own help and our section. */
const GUIDE: Record<
  Guide,
  {
    steps: number;
    links: Partial<Record<number, StepLink>>;
    help?: string;
    docs: string;
  }
> = {
  gmail: {
    steps: 4,
    links: { 1: gmailForwarding },
    help: "https://support.google.com/mail/answer/10957",
    docs: "#gmail",
  },
  workspace: {
    steps: 4,
    links: {
      1: gmailForwarding,
      2: {
        key: "googleAdmin",
        url: () => "https://admin.google.com/ac/apps/gmail/routing",
      },
    },
    help: "https://support.google.com/a/answer/6297084",
    docs: "#google-workspace",
  },
  microsoft: {
    steps: 4,
    links: {
      1: {
        key: "defender",
        url: () => "https://security.microsoft.com/antispam",
      },
      2: {
        key: "exchange",
        url: () => "https://admin.exchange.microsoft.com/#/mailboxes",
      },
      4: {
        key: "outlookForwarding",
        url: () => "https://outlook.office.com/mail/options/mail/forwarding",
      },
    },
    help: "https://learn.microsoft.com/en-us/defender-office-365/outbound-spam-policies-external-email-forwarding",
    docs: "#microsoft-365-and-outlook",
  },
  outlook: {
    steps: 3,
    links: {
      1: {
        key: "outlookCom",
        url: () => "https://outlook.live.com/mail/0/options/mail/forwarding",
      },
    },
    help: "https://support.microsoft.com/en-us/office/turn-on-automatic-forwarding-in-outlook-7f2670a1-7fff-4475-8a3c-5822d63b0c8e",
    docs: "#microsoft-365-and-outlook",
  },
  other: { steps: 2, links: {}, docs: "#any-other-provider" },
};

/** Each host's own forwarding guide, for the Other tab. */
const HOST_HELP: Partial<Record<MailHost, string>> = {
  zoho: "https://www.zoho.com/mail/help/email-forwarding.html",
  yahoo: "https://help.yahoo.com/kb/SLN22028.html",
  icloud: "https://support.apple.com/guide/icloud/mm6b1a3960/icloud",
  fastmail: "https://www.fastmail.help/hc/en-us/articles/360058753434",
  godaddy: "https://www.godaddy.com/help/forward-professional-email-messages-to-another-address-32282",
  namecheap: "https://www.namecheap.com/support/knowledgebase/article.aspx/10784/2306/new-how-to-set-up-forwarding-in-namecheap-private-email/",
  ionos: "https://www.ionos.com/help/email/setting-up-mail-basic/setting-up-forwarding-for-email-addresses/",
  hostinger: "https://www.hostinger.com/support/1583221-how-to-set-up-a-forwarder-for-hostinger-email/",
  ovh: "https://docs.ovhcloud.com/en/guides/web-cloud/email-and-collaborative-solutions/common-email-features/feature-redirections",
  migadu: "https://migadu.com/guides/forwarding/",
  purelymail: "https://purelymail.com/docs/routing",
  rackspace: "https://docs.rackspace.com/docs/set-up-rackspace-email-forwarding",
  yandex: "https://www.yandex.com/support/mail/web/preferences/filters/forwarding.html",
  proton: "https://proton.me/support/email-forwarding",
};

/** The forwarding guide that fits the mailbox's host. */
function forwardingGuide(h: MailHost | null): Guide {
  if (h === "google_workspace") return "workspace";
  if (h === "microsoft365") return "microsoft";
  if (h === "outlook") return "outlook";
  if (h && h !== "gmail") return "other";
  return "gmail";
}

/** Who wrote a guide's help page: Google, Microsoft, or the detected host. */
function helpBy(g: Guide, host: MailHost | null): string {
  if (g === "gmail" || g === "workspace") return "Google";
  if (g === "microsoft" || g === "outlook") return "Microsoft";
  return mailHostLabel(host);
}

export function ForwardingInstructions({ mailbox }: { mailbox: Mailbox }) {
  const { t } = useTranslation("settingsChannels");
  const host = mailboxHost(mailbox);
  const [g, setG] = useState<Guide>(() => forwardingGuide(host));
  const guide = GUIDE[g];
  // A host we know but have no tab for names the Other tab and brings its own guide.
  const otherHost = host && host !== "other" && forwardingGuide(host) === "other" ? host : null;
  const help = g === "other" ? (otherHost ? HOST_HELP[otherHost] : undefined) : guide.help;
  const code = <span className="mono" />;
  const link = "inline-flex items-center gap-1 text-fg-3 underline decoration-line-strong underline-offset-2 hover:text-fg";
  return (
    <div className="flex flex-col gap-4">
      <div>
        <div className="mb-1.5 text-[12.5px] font-medium">
          <Trans t={t} i18nKey="forwarding.forwardTo" values={{ email: mailbox.email }} components={{ code }} />
        </div>
        <CopyField value={mailbox.forwarding_address} />
      </div>
      <div className="flex flex-col gap-3">
        {/* Five providers: in a narrow column the tabs wrap whole, never mid-label. */}
        <Segmented
          size="xs"
          className="flex-wrap self-start [&_button]:whitespace-nowrap"
          value={g}
          onChange={setG}
          options={GUIDES.map((x) => ({
            value: x,
            label: x === "other" && otherHost ? mailHostLabel(otherHost) : t(`forwarding.${x}`),
          }))}
        />
        {g === "microsoft" && (
          <p key="ms" className="flex animate-rise-in gap-2 rounded-md border border-line bg-sunken px-3 py-2.5 text-xs leading-[18px] text-fg-2 motion-reduce:animate-none">
            <TriangleAlert className="mt-px size-3.5 shrink-0 text-tone-amber" aria-hidden />
            <span>{t("forwarding.microsoftBlocked")}</span>
          </p>
        )}
        <ol key={g} className="flex flex-col gap-2.5">
          {Array.from({ length: guide.steps }, (_, i) => {
            const step = guide.links[i + 1];
            return (
              <li
                key={i}
                className="flex animate-rise-in gap-2.5 text-[12.5px] leading-5 text-fg-2 motion-reduce:animate-none [&_b]:font-medium [&_b]:text-fg"
                style={{ animationDelay: `${i * 30}ms` }}
              >
                <span className="mono flex size-5 shrink-0 items-center justify-center rounded-sm border border-line bg-sunken text-[10.5px] text-fg-3">{i + 1}</span>
                <span className="flex min-w-0 flex-col gap-1.5">
                  <span>
                    <Trans t={t} i18nKey={`forwarding.steps.${g}.s${i + 1}` as "forwarding.steps.gmail.s1"} components={{ b: <b /> }} />
                  </span>
                  {step && <ExternalButton href={step.url(mailbox.email)}>{t(`forwarding.open.${step.key}`)}</ExternalButton>}
                </span>
              </li>
            );
          })}
        </ol>
        <div key={`help-${g}`} className="flex flex-wrap gap-x-4 gap-y-1 pl-[30px] text-xs">
          {help && (
            <a href={help} target="_blank" rel="noreferrer" className={link}>
              {t("forwarding.help", { provider: helpBy(g, otherHost) })}
              <ExternalLink className="size-3" aria-hidden />
            </a>
          )}
          <a href={DOCS + guide.docs} target="_blank" rel="noreferrer" className={link}>
            {t("forwarding.docs")}
            <ExternalLink className="size-3" aria-hidden />
          </a>
        </div>
      </div>
      <div className="flex flex-col gap-2 rounded-md border border-line bg-sunken px-3 py-2.5 text-xs leading-[18px] text-fg-3">
        <p>
          <Trans t={t} i18nKey="forwarding.checkIt" values={{ email: mailbox.email }} components={{ code: <span className="mono text-fg-2" /> }} />
        </p>
        <a href={`mailto:${mailbox.email}?subject=${encodeURIComponent(t("forwarding.testSubject"))}`} className={`${link} self-start`}>
          <Send className="size-3" aria-hidden />
          {t("forwarding.sendTest")}
        </a>
      </div>
    </div>
  );
}
