import { useTranslation } from "react-i18next"

interface SlotChipsProps {
  names: string[]
  max?: number
}

export function SlotChips({ names, max = 3 }: SlotChipsProps) {
  const { t } = useTranslation()
  if (names.length === 0) return null
  const visible = names.slice(0, max)
  const overflow = names.length - visible.length

  return (
    <div className="flex gap-1 overflow-hidden">
      {visible.map((name, i) => (
        <span
          key={`${name}-${i}`}
          className="inline-flex min-w-0 items-center gap-1 rounded-[5px] bg-surface-container-low px-1.5 py-0.5 text-[10.5px] text-on-surface-variant"
        >
          <span
            className="size-1 shrink-0 rounded-full bg-outline-variant"
            aria-hidden
          />
          <span className="truncate">{name}</span>
        </span>
      ))}
      {overflow > 0 && (
        <span className="inline-flex items-center rounded-[5px] border border-dashed border-outline-variant px-1.5 py-0.5 text-[10.5px] font-semibold text-on-surface-variant">
          {t("planner.slot.overflow", { count: overflow })}
        </span>
      )}
    </div>
  )
}
