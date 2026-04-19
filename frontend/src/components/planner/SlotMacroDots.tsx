import type { MacrosResponse } from "@/lib/api/weeks"

interface SlotMacroDotsProps {
  macros?: MacrosResponse
}

export function SlotMacroDots({ macros }: SlotMacroDotsProps) {
  if (!macros) return null
  return (
    <div className="flex items-center justify-between gap-2 font-mono text-[10.5px] text-on-surface-variant tabular-nums">
      <span className="font-heading text-[11.5px] font-bold tracking-tight text-on-surface">
        {Math.round(macros.kcal)} kcal
      </span>
      <span className="flex items-center gap-1.5">
        <Dot
          className="bg-[#c87a5a]"
          label={`P${Math.round(macros.protein)}`}
        />
        <Dot className="bg-[#d4b066]" label={`C${Math.round(macros.carbs)}`} />
        <Dot className="bg-[#6f8a73]" label={`F${Math.round(macros.fat)}`} />
      </span>
    </div>
  )
}

function Dot({ className, label }: { className: string; label: string }) {
  return (
    <span className="flex items-center gap-1">
      <span className={`size-1.5 rounded-full ${className}`} aria-hidden />
      {label}
    </span>
  )
}
