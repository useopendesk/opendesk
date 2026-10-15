// Connect flows that sign in to another service (Gmail, Outlook, MCP tools)
// run in the default browser when opendesk is the desktop app: the result
// arrives here through realtime (contracts/native.md).

import { Globe } from "lucide-react";
import { useTranslation } from "react-i18next";
import { toast } from "@/components/ui/toast";
import { i18n } from "@/i18n";
import { cn } from "@/lib/cn";
import { isDesktop, openInBrowser } from "@/lib/desktop";

/** A calm inline note: this step continues in the browser. Nothing in a browser. */
export function InBrowserNote({ name, opened, className }: { name: string; opened?: boolean; className?: string }) {
  const { t } = useTranslation("shell");
  if (!isDesktop) return null;
  return (
    <div role="status" className={cn("flex animate-rise-in items-start gap-2.5 rounded-lg border border-line bg-sunken px-3 py-2.5 text-[12.5px] leading-[18px] text-fg-2 motion-reduce:animate-none", className)}>
      <Globe className="mt-0.5 size-3.5 shrink-0 text-fg-3" aria-hidden />
      <span>{opened ? t("desktop.inBrowserOpened") : t("desktop.inBrowserBody", { name })}</span>
    </div>
  );
}

/** Opens an app page in the browser and says so. */
export function continueInBrowser(path: string) {
  openInBrowser(path);
  toast.info(i18n.t("desktop.inBrowserOpened", { ns: "shell" }));
}
