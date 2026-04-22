import { Heart, UtensilsCrossed } from "lucide-react"
import { useTranslation } from "react-i18next"

import { cn } from "@/lib/utils"

export type PickerPreset =
  | "all"
  | "favorites"
  | "recents"
  | "mains"
  | "sides"
  | "snacks"
  | "sauces"

interface PickerFiltersProps {
  value: PickerPreset
  onChange: (preset: PickerPreset) => void
  counts?: Partial<Record<PickerPreset, number>>
  onSkipShortcut?: () => void
  canSkip?: boolean
}

// Tag + Role dropdowns are not here — those live in PickerSecondaryFilters.
// This row is just the always-visible high-level presets.
export function PickerFilters({
  value,
  onChange,
  counts,
  onSkipShortcut,
  canSkip,
}: PickerFiltersProps) {
  const { t } = useTranslation()
  const items: { key: PickerPreset; label: string }[] = [
    { key: "favorites", label: t("picker.filter.favorites") },
    { key: "recents", label: t("picker.filter.recents") },
    { key: "mains", label: t("picker.filter.mains") },
    { key: "sides", label: t("picker.filter.sides") },
    { key: "snacks", label: t("picker.filter.snacks") },
    { key: "sauces", label: t("picker.filter.sauces") },
  ]

  return (
    <div className="flex flex-wrap gap-2">
      {items.map((it) => {
        const active = value === it.key
        const count = counts?.[it.key]
        return (
          <button
            key={it.key}
            type="button"
            onClick={() => onChange(active ? "all" : it.key)}
            data-testid={`picker-filter-${it.key}`}
            aria-pressed={active}
            className={cn(
              "inline-flex items-center gap-1.5 rounded-full border px-3.5 py-1.5 font-heading text-[12px] font-semibold transition-colors",
              active
                ? "border-transparent bg-primary text-on-primary"
                : "border-outline-variant bg-surface-container-lowest text-on-surface-variant hover:bg-surface-container-high hover:text-on-surface"
            )}
          >
            {it.key === "favorites" && (
              <Heart
                className={cn("h-3 w-3", active && "fill-current")}
                aria-hidden
              />
            )}
            <span>{it.label}</span>
            {count !== undefined && count > 0 && (
              <span
                className={cn(
                  "rounded-full px-1.5 py-0 text-[10.5px]",
                  active
                    ? "bg-white/25 text-on-primary"
                    : "bg-surface-container-high text-on-surface-variant"
                )}
              >
                {count}
              </span>
            )}
          </button>
        )
      })}
      {onSkipShortcut && canSkip && (
        <button
          type="button"
          onClick={onSkipShortcut}
          data-testid="picker-filter-skip"
          className="ml-auto inline-flex items-center gap-1.5 rounded-full border border-tertiary/40 bg-white px-3.5 py-1.5 font-heading text-[12px] font-semibold text-tertiary hover:bg-tertiary/10"
        >
          <UtensilsCrossed className="h-3 w-3" aria-hidden />
          {t("picker.filter.skip")}
        </button>
      )}
    </div>
  )
}
