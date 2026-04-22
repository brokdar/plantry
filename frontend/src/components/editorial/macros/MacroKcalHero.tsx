import { useTranslation } from "react-i18next"

import { AnimatedNumber } from "@/components/editorial/AnimatedNumber"
import { cn } from "@/lib/utils"

interface MacroKcalHeroProps {
  kcal: number | null | undefined
  hint?: string
  size?: "sm" | "md" | "lg"
  align?: "start" | "center"
  className?: string
}

const SIZE: Record<
  NonNullable<MacroKcalHeroProps["size"]>,
  {
    value: string
    unit: string
  }
> = {
  sm: {
    value: "font-heading text-2xl font-extrabold leading-none",
    unit: "text-[11px]",
  },
  md: {
    value: "font-heading text-3xl font-extrabold leading-none",
    unit: "text-xs",
  },
  lg: {
    value: "font-heading text-5xl font-extrabold leading-none",
    unit: "text-sm",
  },
}

export function MacroKcalHero({
  kcal,
  hint,
  size = "md",
  align = "start",
  className,
}: MacroKcalHeroProps) {
  const { t } = useTranslation()
  const typo = SIZE[size]

  return (
    <div
      className={cn(
        "flex flex-col gap-1",
        align === "center" && "items-center text-center",
        className
      )}
      data-testid="macro-kcal-hero"
    >
      <p className={cn("text-on-surface tabular-nums", typo.value)}>
        {kcal == null ? (
          "—"
        ) : (
          <AnimatedNumber
            value={kcal}
            format={(n) => Math.round(n).toLocaleString()}
          />
        )}
        <span
          className={cn(
            "ml-1.5 font-medium tracking-[0.18em] text-on-surface-variant uppercase",
            typo.unit
          )}
        >
          {t("macro.kcal")}
        </span>
      </p>
      {hint && (
        <p className="text-[10px] tracking-[0.18em] text-on-surface-variant/80 uppercase">
          {hint}
        </p>
      )}
    </div>
  )
}
