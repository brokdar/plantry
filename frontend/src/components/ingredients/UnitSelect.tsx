import { useTranslation } from "react-i18next"
import { Select as SelectPrimitive } from "radix-ui"
import { CheckIcon } from "lucide-react"

import {
  Select,
  SelectContent,
  SelectGroup,
  SelectLabel,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { cn } from "@/lib/utils"
import {
  normalizeUnit,
  unitGroups,
  type PortionLookup,
  type UnitGroup,
  type UnitOption,
} from "@/lib/domain/units"

interface UnitSelectProps {
  value: string
  onValueChange: (canonical: string) => void
  portions?: PortionLookup[]
  /** Canonical keys to hide (e.g. already-used portion units). */
  excludeKeys?: string[]
  disabled?: boolean
  placeholder?: string
  testId?: string
  className?: string
}

const GROUP_ORDER: UnitGroup[] = [
  "portions",
  "mass",
  "volume",
  "count",
  "custom",
]

export function UnitSelect({
  value,
  onValueChange,
  portions,
  excludeKeys,
  disabled,
  placeholder,
  testId,
  className,
}: UnitSelectProps) {
  const { t } = useTranslation()
  const groups = unitGroups(portions ?? [])
  const excluded = new Set(excludeKeys ?? [])
  const filter = (items: UnitOption[]) =>
    excluded.size === 0 ? items : items.filter((o) => !excluded.has(o.key))

  const allOptions = GROUP_ORDER.flatMap((g) => filter(groups[g]))

  const normalized = normalizeUnit(value)
  const hasValue = allOptions.some((o) => o.key === normalized)
  const fallbackCustom: UnitOption | null =
    !hasValue && normalized && !excluded.has(normalized)
      ? { key: normalized, group: "custom" }
      : null

  return (
    <Select
      value={value}
      onValueChange={(v) => onValueChange(normalizeUnit(v))}
      disabled={disabled}
    >
      <SelectTrigger
        data-testid={testId}
        className={cn("w-full justify-between", className)}
      >
        <SelectValue placeholder={placeholder ?? t("unit.placeholder")} />
      </SelectTrigger>
      <SelectContent className="max-h-80">
        {GROUP_ORDER.map((group) => {
          const base = filter(groups[group])
          const items =
            group === "custom" && fallbackCustom
              ? [fallbackCustom, ...base]
              : base
          if (items.length === 0) return null
          return (
            <SelectGroup key={group}>
              <SelectLabel className="px-2.5 pt-2 pb-1 font-heading text-[10px] tracking-[0.18em] text-on-surface-variant/80 uppercase">
                {t(`unit.group.${group}`)}
              </SelectLabel>
              {items.map((opt) => (
                <UnitItem key={opt.key} option={opt} />
              ))}
            </SelectGroup>
          )
        })}
      </SelectContent>
    </Select>
  )
}

function UnitItem({ option }: { option: UnitOption }) {
  const { t } = useTranslation()
  const name = t(`unit.${option.key}.name`, { defaultValue: option.key })
  const gramsHint =
    option.grams !== undefined
      ? t("unit.exact_grams", { grams: formatGrams(option.grams) })
      : null
  // Render the grams hint as a sibling of ItemText so it appears in the
  // dropdown row but does NOT get portaled into the trigger (Radix portals
  // only the ItemText content into the selected-value display).
  return (
    <SelectPrimitive.Item
      value={option.key}
      textValue={option.key}
      data-slot="select-item"
      data-testid={`unit-option-${option.key}`}
      className="relative flex w-full cursor-default items-center gap-2.5 rounded-md py-1.5 pr-8 pl-1.5 text-sm outline-hidden select-none focus:bg-accent focus:text-accent-foreground data-disabled:pointer-events-none data-disabled:opacity-50"
    >
      <span className="pointer-events-none absolute right-2 flex size-4 items-center justify-center">
        <SelectPrimitive.ItemIndicator>
          <CheckIcon className="size-4" />
        </SelectPrimitive.ItemIndicator>
      </span>
      <SelectPrimitive.ItemText>
        <span className="flex items-baseline gap-2.5">
          <span className="min-w-[2.25rem] shrink-0 font-mono text-[11px] tracking-tight text-on-surface tabular-nums">
            {option.key}
          </span>
          <span className="text-sm text-on-surface">{name}</span>
        </span>
      </SelectPrimitive.ItemText>
      {gramsHint && (
        <span className="ml-auto shrink-0 font-mono text-[10px] tracking-tight text-on-surface-variant/80">
          {gramsHint}
        </span>
      )}
    </SelectPrimitive.Item>
  )
}

/**
 * Inline symbol + localized name badge. Shared between UnitSelect items and
 * any surface (e.g. PortionsEditor row) that lists units outside a dropdown.
 */
export function UnitLabel({
  unit,
  className,
}: {
  unit: string
  className?: string
}) {
  const { t } = useTranslation()
  const key = normalizeUnit(unit)
  const name = t(`unit.${key}.name`, { defaultValue: unit })
  return (
    <span className={cn("flex items-baseline gap-2.5", className)}>
      <span className="min-w-[2.25rem] shrink-0 font-mono text-[11px] tracking-tight text-on-surface-variant tabular-nums">
        {key || unit}
      </span>
      <span className="text-sm text-on-surface">{name}</span>
    </span>
  )
}

function formatGrams(g: number): string {
  if (g >= 100) return Math.round(g).toString()
  if (g >= 10) return g.toFixed(0)
  return g.toFixed(1).replace(/\.0$/, "")
}
