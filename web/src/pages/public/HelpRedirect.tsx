// /help/<slug>/...: help centers are rendered by the API (contracts/teams.md "Help
// center addresses"), so an old app link goes on to the help center itself. With
// helpDomain that is https://<slug>.<helpDomain> (`/help/<slug>/c/x` is `/c/x`
// there); without it the API binary serves /help/<slug>, on the API's origin when the
// app is hosted elsewhere, else on this one, so a full page load reaches it. A load
// that comes back here again (the dev server, or a host that does not route /help to
// the API) stops and explains instead.

import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { RoutePending } from "@/app/RouteStates";
import { Button } from "@/components/ui/button";
import { EmptyState } from "@/components/ui/empty";
import { config } from "@/lib/config";
import { legacyHelpTarget } from "@/lib/helpCenter";

const RELOAD_KEY = "od.help-reload";

/** Whether this page asked for this same address a moment ago and got the app back. */
function cameBack(href: string): boolean {
  try {
    const v = JSON.parse(sessionStorage.getItem(RELOAD_KEY) ?? "null") as { href?: string; at?: number } | null;
    return v?.href === href && Date.now() - (v.at ?? 0) < 10_000;
  } catch {
    return false;
  }
}

function markReload(href: string) {
  try {
    sessionStorage.setItem(RELOAD_KEY, JSON.stringify({ href, at: Date.now() }));
  } catch {
    // Without storage the reload still happens; the dev check below covers the dev server.
  }
}

/** The same page on the API's own origin, when that is somewhere else (a separate API host in development). */
function apiHref(): string | null {
  if (!config.apiUrl) return null;
  try {
    const api = new URL(config.apiUrl, window.location.href);
    if (api.origin === window.location.origin) return null;
    return `${api.origin}${window.location.pathname}${window.location.search}${window.location.hash}`;
  } catch {
    return null;
  }
}

export default function HelpRedirect() {
  const { t } = useTranslation("shell");
  // Without helpDomain the API renders /help/<slug>: on its own origin when that is elsewhere
  // (the app on a static host), else here after a full load.
  const [target] = useState(() => legacyHelpTarget(window.location) ?? (import.meta.env.DEV ? null : apiHref()));
  // The dev server answers /help with the app again, so it never reloads there.
  const [explain] = useState(() => !target && (import.meta.env.DEV || cameBack(window.location.href)));

  useEffect(() => {
    if (target) window.location.replace(target);
    else if (!explain) {
      markReload(window.location.href);
      window.location.replace(window.location.href);
    }
  }, [target, explain]);

  if (!explain) return <RoutePending />;
  const href = apiHref();
  return (
    <div className="flex min-h-screen items-center justify-center p-8">
      <EmptyState
        art="kb"
        title={t("helpRedirect.title")}
        description={import.meta.env.DEV ? t("helpRedirect.devBody") : t("helpRedirect.body")}
        action={
          href ? (
            <Button variant="secondary" asChild>
              <a href={href}>{t("helpRedirect.openApi")}</a>
            </Button>
          ) : undefined
        }
        tips={import.meta.env.DEV && !href ? [{ label: t("helpRedirect.devTip") }] : undefined}
      />
    </div>
  );
}
