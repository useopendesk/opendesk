// Quiet lines above the mail list (contracts/collab.md "Search", "Storage"): how far back a
// search reaches, with a way to import older mail, and that new mail waits at the provider
// while the workspace's Collab storage is full.

import { Link } from "@tanstack/react-router";
import { HardDrive, History } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { MailAccount } from "@/lib/api/types";
import { useCollab } from "@/lib/collab/hooks";
import { cn } from "@/lib/cn";
import { longDate } from "@/lib/format";
import { atLeast, useRole } from "@/lib/session";
import { tone } from "@/lib/tones";

/**
 * The date from which every account in view has its mail here: the latest of their oldest
 * stored mail, so the line never promises more than the least covered account has.
 */
export function searchCoveredFrom(accounts: MailAccount[]): string | null {
  let out: string | null = null;
  for (const a of accounts) if (a.oldest_message_at && (!out || a.oldest_message_at > out)) out = a.oldest_message_at;
  return out;
}

/** An account whose older mail could still come in: no finished import of all its mail. */
const hasMoreAtProvider = (a: MailAccount) => !(a.history?.status === "done" && a.history.since === null);

const linkCls = "inline-flex items-center gap-1 font-medium text-fg-2 underline decoration-line-strong underline-offset-2 hover:text-fg hover:decoration-fg-3 [&_svg]:size-3";

export function SearchCoverage({ accounts }: { accounts: MailAccount[] }) {
  const { t } = useTranslation("collab");
  const from = searchCoveredFrom(accounts);
  if (!from) return null;
  const only = accounts.length === 1 ? accounts[0] : undefined;
  return (
    <div className="flex h-8 shrink-0 items-center gap-2 border-b border-line-soft px-4 text-[11.5px] text-fg-3">
      <span className="min-w-0 truncate">{t("list.searchCoverage", { date: longDate(from) })}</span>
      {only?.can_manage && hasMoreAtProvider(only) && (
        <Link to="/settings/mail-accounts/$accountId" params={{ accountId: only.id }} className={cn(linkCls, "ml-auto shrink-0")}>
          <History />
          {t("list.searchImport")}
        </Link>
      )}
    </div>
  );
}

export function StorageFullBanner() {
  const { t } = useTranslation("collab");
  const { status } = useCollab();
  const admin = atLeast(useRole(), "admin");
  if (!status?.storage?.full) return null;
  return (
    <div role="status" className={cn("flex shrink-0 items-start gap-2 border-b border-line-soft px-4 py-2 text-[11.5px] leading-[17px] animate-rise-in motion-reduce:animate-none", tone.amber.soft)}>
      <HardDrive className={cn("mt-px size-3.5 shrink-0", tone.amber.text)} aria-hidden />
      <p className="min-w-0 text-fg-2">
        {t("list.storageFull")}
        {admin && (
          <>
            {" "}
            {t("list.storageFullAdmin")}{" "}
            <Link to="/settings/billing" search={{}} className={linkCls}>
              {t("list.billing")}
            </Link>
          </>
        )}
      </p>
    </div>
  );
}
