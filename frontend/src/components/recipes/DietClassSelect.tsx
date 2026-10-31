import { useTranslation } from "react-i18next"

import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
} from "@/components/ui/select"
import type { DietClass, EffectiveDietClass, Food } from "@/lib/api/foods"
import { useSetDietClass } from "@/lib/queries/foods"
import { DIET_CLASSES } from "@/lib/schemas/food"
import { cn } from "@/lib/utils"

import { DietIcon } from "./DietIcon"

const AUTO = "auto"

type DietClassSelectProps = {
  /** The user's override; null = automatic. */
  value: DietClass | null
  /** The class shown while automatic; none before the food exists. */
  effective?: EffectiveDietClass
  onChange: (next: DietClass | null) => void
  /** The trigger's accessible name. */
  label: string
  disabled?: boolean
  /** Below sm only the icon shows (the name stays the accessible text). */
  compact?: boolean
  className?: string
  testId?: string
}

/** Picks a diet class override (D28). "Automatic" drops the override and
 *  hands the food back to the classifier (ingredient) or its ingredients
 *  (recipe). */
export function DietClassSelect({
  value,
  effective,
  onChange,
  label,
  disabled,
  compact,
  className,
  testId,
}: DietClassSelectProps) {
  const { t } = useTranslation()
  const shown = value ?? effective

  return (
    <Select
      value={value ?? AUTO}
      onValueChange={(v) => onChange(v === AUTO ? null : (v as DietClass))}
      disabled={disabled}
    >
      <SelectTrigger
        aria-label={label}
        className={cn(
          "min-h-11 rounded-cell border-line bg-transparent text-body data-[size=default]:h-auto md:pointer-fine:min-h-8",
          className
        )}
        data-testid={testId}
      >
        <span className="flex items-center gap-1.5 text-left">
          {shown ? (
            <DietIcon
              dietClass={shown}
              withLabel
              className={cn(compact && "[&>span]:max-sm:sr-only")}
            />
          ) : (
            t("diet.auto")
          )}
          {value === null && shown && (
            <span className="text-caption text-ink-muted max-sm:sr-only">
              {t("diet.auto_hint")}
            </span>
          )}
        </span>
      </SelectTrigger>
      <SelectContent position="popper">
        <SelectItem value={AUTO}>{t("diet.auto")}</SelectItem>
        {DIET_CLASSES.map((c) => (
          <SelectItem key={c} value={c}>
            <DietIcon dietClass={c} withLabel />
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

/** A food's diet class, saved on change (`PUT /api/foods/{id}/diet-class`). */
export function FoodDietSelect({
  food,
  ...rest
}: { food: Food } & Pick<
  DietClassSelectProps,
  "label" | "compact" | "className" | "testId"
>) {
  const setDiet = useSetDietClass()
  const override =
    food.diet_class_source === "user" && food.diet_class !== "unknown"
      ? food.diet_class
      : null
  return (
    <DietClassSelect
      value={override}
      effective={food.diet_class}
      onChange={(dietClass) => setDiet.mutate({ id: food.id, dietClass })}
      disabled={setDiet.isPending}
      {...rest}
    />
  )
}
