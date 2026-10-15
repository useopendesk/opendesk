// Brands in the knowledge base (contracts/teams.md): a collection or article belongs
// to one brand, or to every brand (brand_id null). An article in a collection with a
// brand belongs to the collection's brand. Brand UI shows once there is a second brand.

import { Layers } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { KbArticle, KbCollection } from "@/lib/api/types";
import type { PickerItem } from "@/components/ui/picker";
import { SinglePick, type PickSize } from "@/components/settings2/pickers";
import { brandItems, useBrands } from "@/components/settings2/teams";

export const ALL_BRANDS = "__all";

/** The brand an article belongs to: its collection's when that has one, else its own; null = every brand. */
export function articleBrandId(a: Pick<KbArticle, "brand_id" | "collection_id">, collections: KbCollection[] | undefined): string | null {
  const col = a.collection_id ? collections?.find((c) => c.id === a.collection_id) : undefined;
  return col?.brand_id ?? a.brand_id ?? null;
}

const everyBrandIcon = <Layers className="text-fg-3" />;

/** "All brands" or one brand, as a builder-style value button. */
export function KbBrandPick({
  value,
  onChange,
  size,
  disabled,
  className,
  filter,
}: {
  value: string | null | undefined;
  onChange: (id: string | null) => void;
  size?: PickSize;
  disabled?: boolean;
  className?: string;
  /** As a list filter, where "All brands" means everything rather than shared. */
  filter?: boolean;
}) {
  const { t } = useTranslation("kb");
  const { brands } = useBrands();
  const items: PickerItem[] = [{ value: ALL_BRANDS, label: t("brand.all"), icon: everyBrandIcon, hint: filter ? undefined : t("brand.allHint") }, ...brandItems(brands)];
  return (
    <SinglePick
      items={items}
      value={value ?? ALL_BRANDS}
      onChange={(v) => onChange(v === ALL_BRANDS ? null : v)}
      placeholder={t("brand.all")}
      searchPlaceholder={t("brand.search")}
      size={size}
      disabled={disabled}
      className={className}
    />
  );
}
