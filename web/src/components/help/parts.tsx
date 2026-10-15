import { Link, useNavigate } from "@tanstack/react-router";
import { ArrowRight, ChevronRight, FileText, Search } from "lucide-react";
import { useState, type CSSProperties } from "react";
import { useTranslation } from "react-i18next";
import type { KbArticle, KbCollection } from "@/lib/api/types";
import { cn } from "@/lib/cn";
import { shortDate } from "@/lib/format";
import { tone, toneFor } from "@/lib/tones";

/**
 * The help center's mark: its logo (the brand's help center logo, else the brand logo,
 * else the workspace's; the API picks), or the first letter on the name's hue.
 */
export function HelpGlyph({ name, src, className }: { name: string; src?: string | null; className?: string }) {
  const [broken, setBroken] = useState<string | null>(null);
  if (src && broken !== src)
    return <img src={src} alt="" onError={() => setBroken(src)} className={cn("size-6 shrink-0 rounded-md object-contain", className)} />;
  return (
    <span
      aria-hidden
      className={cn("flex size-6 shrink-0 items-center justify-center rounded-md text-[11px] font-semibold text-white", tone[toneFor(name)].solid, className)}
    >
      {name.trim().charAt(0).toUpperCase() || "?"}
    </span>
  );
}

export function HelpSearchBox({ slug, initial = "", size = "md", autoFocus }: { slug: string; initial?: string; size?: "md" | "lg"; autoFocus?: boolean }) {
  const { t } = useTranslation("public");
  const [value, setValue] = useState(initial);
  const navigate = useNavigate();
  return (
    <form
      role="search"
      onSubmit={(e) => {
        e.preventDefault();
        const v = value.trim();
        if (v) void navigate({ to: "/help/$slug/search", params: { slug }, search: { q: v } });
      }}
      className="group/search relative w-full"
    >
      <Search
        className={cn(
          "pointer-events-none absolute top-1/2 -translate-y-1/2 text-fg-4 transition-colors group-focus-within/search:text-fg-2",
          size === "lg" ? "start-4 size-4.5" : "start-2.5 size-3.5",
        )}
      />
      <input
        type="search"
        value={value}
        autoFocus={autoFocus}
        onChange={(e) => setValue(e.target.value)}
        placeholder={t("help.search_placeholder")}
        aria-label={t("help.search_placeholder")}
        className={cn(
          "w-full border border-line bg-surface text-fg outline-none transition-[border-color,box-shadow] duration-150 placeholder:text-fg-4 hover:border-line-strong focus:border-fg-4 focus:shadow-[0_0_0_3px_var(--focus)] [&::-webkit-search-cancel-button]:hidden",
          size === "lg" ? "h-12 rounded-lg ps-11 pe-4 text-[15px] shadow-xs" : "h-8 rounded-md ps-8 pe-3 text-[13px]",
        )}
      />
    </form>
  );
}

export function CollectionCard({ slug, c, index = 0 }: { slug: string; c: KbCollection; index?: number }) {
  const { t } = useTranslation("public");
  const hue = tone[toneFor(c.name)];
  return (
    <Link
      to="/help/$slug/c/$collection"
      params={{ slug, collection: c.slug }}
      preload="intent"
      style={{ animationDelay: `${Math.min(index, 8) * 35}ms` } as CSSProperties}
      className="group relative flex animate-rise-in flex-col rounded-lg border border-line bg-surface p-4 transition-[border-color,box-shadow,transform] duration-150 hover:border-line-strong hover:shadow-xs active:scale-[0.995] motion-reduce:animate-none"
    >
      <span className={cn("mb-4 flex size-8 items-center justify-center rounded-md", hue.chip)} aria-hidden>
        <FileText className="size-4" strokeWidth={1.75} />
      </span>
      <span className="text-[14px] font-semibold tracking-[-0.015em] text-fg">
        <bdi>{c.name}</bdi>
      </span>
      {c.description && (
        <span className="mt-1 line-clamp-2 text-[13px] leading-5 text-fg-3">
          <bdi>{c.description}</bdi>
        </span>
      )}
      <span className="mt-auto flex items-center gap-1 pt-4 text-[12px] text-fg-4 transition-colors group-hover:text-fg-2">
        {t("help.articles", { count: c.article_count })}
        <ArrowRight className="size-3 -translate-x-0.5 opacity-0 transition-all duration-150 group-hover:translate-x-0 group-hover:opacity-100 rtl:-scale-x-100" />
      </span>
    </Link>
  );
}

export function ArticleRow({ slug, article, compact }: { slug: string; article: KbArticle; compact?: boolean }) {
  return (
    <Link
      to="/help/$slug/a/$article"
      params={{ slug, article: article.slug }}
      preload="intent"
      className={cn(
        "group -mx-3 flex items-start gap-3 rounded-md px-3 transition-colors duration-150 hover:bg-sunken",
        compact ? "py-2.5" : "py-3.5",
      )}
    >
      <FileText className="mt-0.5 size-4 shrink-0 text-fg-4 transition-colors group-hover:text-fg-2" strokeWidth={1.75} />
      <span className="min-w-0 flex-1">
        <span className="block text-[14px] font-medium tracking-[-0.01em] text-fg">
          <bdi>{article.title}</bdi>
        </span>
        {!compact && article.excerpt && (
          <span className="mt-0.5 line-clamp-2 block text-[13px] leading-5 text-fg-3">
            <bdi>{article.excerpt}</bdi>
          </span>
        )}
      </span>
      {!compact && <span className="mt-0.5 hidden shrink-0 text-[12px] tabular-nums text-fg-4 sm:block">{shortDate(article.published_at ?? article.updated_at)}</span>}
      <ArrowRight className="mt-0.5 size-3.5 shrink-0 -translate-x-1 text-fg-3 opacity-0 transition-all duration-150 group-hover:translate-x-0 group-hover:opacity-100 rtl:-scale-x-100 rtl:translate-x-1" />
    </Link>
  );
}

/** Rows separated by hairlines, without boxing them in. */
export function ArticleList({ slug, articles, compact }: { slug: string; articles: KbArticle[]; compact?: boolean }) {
  return (
    <ul className="flex flex-col">
      {articles.map((a, i) => (
        <li
          key={a.id}
          className="animate-rise-in border-b border-line-soft last:border-0 motion-reduce:animate-none"
          style={{ animationDelay: `${Math.min(i, 8) * 30}ms` }}
        >
          <ArticleRow slug={slug} article={a} compact={compact} />
        </li>
      ))}
    </ul>
  );
}

export function Crumbs({ slug, items }: { slug: string; items: { label: string; to?: { collection: string } }[] }) {
  const { t } = useTranslation("public");
  return (
    <nav aria-label={t("help.breadcrumb")} className="mb-6 flex flex-wrap items-center gap-1 text-[12.5px] text-fg-3">
      <Link to="/help/$slug" params={{ slug }} className="rounded-sm transition-colors hover:text-fg">
        {t("help.default_title")}
      </Link>
      {items.map((i) => (
        <span key={i.label} className="flex min-w-0 items-center gap-1">
          <ChevronRight className="size-3 shrink-0 text-fg-4 rtl:-scale-x-100" />
          {i.to ? (
            <Link to="/help/$slug/c/$collection" params={{ slug, collection: i.to.collection }} preload="intent" className="rounded-sm transition-colors hover:text-fg">
              <bdi>{i.label}</bdi>
            </Link>
          ) : (
            <bdi className="truncate text-fg-2" aria-current="page">
              {i.label}
            </bdi>
          )}
        </span>
      ))}
    </nav>
  );
}
