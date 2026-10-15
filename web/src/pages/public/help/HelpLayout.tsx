import { useQuery } from "@tanstack/react-query";
import { useEffect } from "react";
import { getRouteApi, Link, Outlet, useRouterState } from "@tanstack/react-router";
import { Moon, Sun } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Lockup } from "@/components/brand";
import { EmptyState } from "@/components/ui/empty";
import { HelpGlyph, HelpSearchBox } from "@/components/help/parts";
import { LanguageSwitcher, usePublicLocale } from "@/components/help/locale";
import "@/components/help/help.css";
import { useDocumentTitle } from "@/app/RootLayout";
import { q } from "@/lib/queries";
import { cn } from "@/lib/cn";
import { useTheme } from "@/lib/theme";

const route = getRouteApi("/help/$slug");

export default function HelpLayout() {
  const { slug } = route.useParams();
  const { data, isError, isPending } = useQuery(q.publicHelp(slug));
  const { locale, loaded } = usePublicLocale(data?.locale || undefined, !isPending);
  const { t } = useTranslation("public");
  const { dark, setTheme } = useTheme();
  const path = useRouterState({ select: (s) => s.location.pathname });
  const atHome = path.replace(/\/+$/, "") === `/help/${slug}`;
  const title = data?.title || t("help.default_title");
  useDocumentTitle(atHome && loaded ? title : null);
  useFavicon(data?.favicon_url);

  return (
    <div data-public-page className="flex min-h-dvh flex-col bg-surface text-fg">
      <header
        className={cn(
          "sticky top-0 z-10 border-b backdrop-blur-md transition-colors duration-200",
          atHome ? "border-transparent bg-sunken/90" : "border-line-soft bg-surface/90",
        )}
      >
        <div className={cn("mx-auto flex h-14 items-center gap-4 px-5", atHome ? "max-w-[960px]" : "max-w-[760px]")}>
          <Link to="/help/$slug" params={{ slug }} className="flex min-w-0 items-center gap-2.5 rounded-md">
            {loaded && <HelpGlyph name={title} src={data?.logo_url} />}
            <bdi className="truncate text-[14px] font-semibold tracking-[-0.015em]">{loaded ? title : ""}</bdi>
          </Link>
          <div className="ms-auto flex items-center gap-2">
            {!atHome && loaded && (
              <div className="hidden w-[240px] sm:block">
                <HelpSearchBox key={path} slug={slug} />
              </div>
            )}
            <button
              type="button"
              aria-label={dark ? t("help.theme_light") : t("help.theme_dark")}
              onClick={() => setTheme(dark ? "light" : "dark")}
              className="flex size-8 items-center justify-center rounded-md text-fg-3 transition-colors hover:bg-hover hover:text-fg active:scale-[0.96]"
            >
              {dark ? <Sun className="size-4" /> : <Moon className="size-4" />}
            </button>
          </div>
        </div>
      </header>
      <main className="flex flex-1 flex-col">
        {isError ? (
          loaded && <EmptyState lit={2} title={t("help.not_found_title")} description={t("help.not_found_body")} />
        ) : (
          // Hold the page until its language is in, so English never flashes first.
          loaded && <Outlet />
        )}
      </main>
      <footer className="border-t border-line-soft">
        <div className={cn("mx-auto flex h-14 items-center gap-3 px-5 text-xs text-fg-4", atHome ? "max-w-[960px]" : "max-w-[760px]")}>
          <span className="min-w-0 truncate">
            © {new Date().getFullYear()} {data?.title ?? ""}
          </span>
          <LanguageSwitcher current={locale} className="ms-auto" />
          <a href="https://opendesk.sh" className="flex items-center gap-1.5 rounded-md transition-colors hover:text-fg-2">
            {t("footer.powered_by")} <Lockup className="h-[11px]" />
          </a>
        </div>
      </footer>
    </div>
  );
}

/** The brand's help center favicon while the help center is open; the app's icon comes back after. */
function useFavicon(href: string | null | undefined) {
  useEffect(() => {
    if (!href) return;
    const links = [...document.querySelectorAll<HTMLLinkElement>('link[rel~="icon"]')];
    const before = links.map((l) => [l, l.href, l.type] as const);
    let added: HTMLLinkElement | null = null;
    if (links.length) {
      for (const l of links) {
        l.href = href;
        l.removeAttribute("type");
      }
    } else {
      added = Object.assign(document.createElement("link"), { rel: "icon", href });
      document.head.appendChild(added);
    }
    return () => {
      for (const [l, h, type] of before) {
        l.href = h;
        if (type) l.type = type;
      }
      added?.remove();
    };
  }, [href]);
}
