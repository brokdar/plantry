import { useTranslation } from "react-i18next"

import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { HEATS, type Heat } from "@/lib/api/kitchen"

import { OptionCards } from "./controls"

const MINUTE_CHOICES = [15, 20, 30, 45, 60, 90, 120, 180]

/** The active cooking time a cook may spend: a few usual values, plus the
 *  stored one when it is another. */
export function MinutesPicker({
  id,
  value,
  onChange,
}: {
  id: string
  value: number
  onChange: (next: number) => void
}) {
  const { t } = useTranslation()
  const choices = MINUTE_CHOICES.includes(value)
    ? MINUTE_CHOICES
    : [...MINUTE_CHOICES, value].sort((a, b) => a - b)
  return (
    <Select value={String(value)} onValueChange={(v) => onChange(Number(v))}>
      <SelectTrigger
        id={id}
        className="w-full max-w-48 pointer-coarse:h-11"
        data-testid={id}
      >
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        <SelectGroup>
          {choices.map((n) => (
            <SelectItem key={n} value={String(n)}>
              {t("settings_page.kitchen.minutes", { count: n })}
            </SelectItem>
          ))}
        </SelectGroup>
      </SelectContent>
    </Select>
  )
}

/** The heat tolerance; null is "not set" and nothing is selected. */
export function HeatPicker({
  name,
  value,
  onChange,
}: {
  name: string
  value: Heat | null
  onChange: (next: Heat) => void
}) {
  const { t } = useTranslation()
  return (
    <OptionCards
      name={name}
      legend={t("settings_page.kitchen.heat_label")}
      options={HEATS.map((h) => ({
        value: h,
        label: t(`settings_page.kitchen.heat.${h}`),
      }))}
      value={value}
      onChange={onChange}
    />
  )
}
