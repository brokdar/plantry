import {
  CircleQuestionMark,
  Drumstick,
  Egg,
  Fish,
  Sprout,
  type LucideIcon,
} from "lucide-react"
import { useTranslation } from "react-i18next"

import type { EffectiveDietClass } from "@/lib/api/foods"
import { cn } from "@/lib/utils"

const ICONS: Record<EffectiveDietClass, LucideIcon> = {
  vegan: Sprout,
  vegetarian: Egg,
  pescetarian: Fish,
  omnivore: Drumstick,
  unknown: CircleQuestionMark,
}

type DietIconProps = {
  dietClass: EffectiveDietClass
  /** Shows the class name next to the icon; the icon is then decorative. */
  withLabel?: boolean
  className?: string
}

/** A diet class (D28) as an icon. Each class has its own shape and a text
 *  label (visible or as the accessible name), never colour alone. */
export function DietIcon({ dietClass, withLabel, className }: DietIconProps) {
  const { t } = useTranslation()
  const Icon = ICONS[dietClass]
  const name = t(`diet.class.${dietClass}`)

  if (withLabel) {
    return (
      <span
        className={cn("inline-flex items-center gap-1.5", className)}
        data-diet={dietClass}
      >
        <Icon className="size-4 shrink-0" strokeWidth={1.5} aria-hidden />
        <span>{name}</span>
      </span>
    )
  }
  const label = t("diet.icon_label", { class: name })
  return (
    <span
      role="img"
      aria-label={label}
      title={label}
      className={cn("inline-flex items-center", className)}
      data-diet={dietClass}
    >
      <Icon className="size-4 shrink-0" strokeWidth={1.5} aria-hidden />
    </span>
  )
}
