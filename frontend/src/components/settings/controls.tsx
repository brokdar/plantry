import { Check, Minus, Plus } from "lucide-react"
import { useId, useState } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { cn } from "@/lib/utils"

const touchSize = "size-9 pointer-coarse:size-11"

/** − value + for small whole-number ranges. */
export function Stepper({
  label,
  value,
  min,
  max,
  onChange,
  format,
  disabled,
  id,
}: {
  /** Names the value for the − / + buttons ("Decrease adults"). */
  label: string
  value: number
  min: number
  max: number
  onChange: (next: number) => void
  format?: (value: number) => string
  disabled?: boolean
  id?: string
}) {
  const { t } = useTranslation()
  return (
    <div className="flex items-center gap-2" role="group" aria-label={label}>
      <Button
        type="button"
        variant="outline"
        className={touchSize}
        aria-label={t("settings_page.decrease", { label })}
        disabled={disabled || value <= min}
        onClick={() => onChange(Math.max(min, value - 1))}
      >
        <Minus strokeWidth={1.5} aria-hidden />
      </Button>
      <output
        id={id}
        aria-live="polite"
        className="min-w-16 text-center text-body text-ink tabular-nums"
      >
        {format ? format(value) : value}
      </output>
      <Button
        type="button"
        variant="outline"
        className={touchSize}
        aria-label={t("settings_page.increase", { label })}
        disabled={disabled || value >= max}
        onClick={() => onChange(Math.min(max, value + 1))}
      >
        <Plus strokeWidth={1.5} aria-hidden />
      </Button>
    </div>
  )
}

/** An on/off switch built on a native checkbox. */
export function Switch({
  checked,
  onChange,
  label,
  id,
  testId,
}: {
  checked: boolean
  onChange: (next: boolean) => void
  /** Accessible name when no visible label points at `id`. */
  label?: string
  id?: string
  testId?: string
}) {
  return (
    <span className="relative inline-flex h-11 w-12 shrink-0 items-center md:h-9 pointer-coarse:h-11">
      <input
        id={id}
        type="checkbox"
        role="switch"
        aria-label={label}
        checked={checked}
        onChange={(e) => onChange(e.target.checked)}
        data-testid={testId}
        className="peer absolute inset-0 z-10 cursor-pointer appearance-none rounded-full"
      />
      <span
        aria-hidden
        className="h-6 w-11 rounded-full border border-line bg-panel-sunken transition-colors duration-150 ease-out peer-checked:border-accent peer-checked:bg-accent"
      />
      <span
        aria-hidden
        className="absolute left-1 size-4 rounded-full bg-ink-muted transition-transform duration-150 ease-out peer-checked:translate-x-5 peer-checked:bg-on-accent"
      />
    </span>
  )
}

/** A whole-number input that saves on blur when the value is in range. */
export function NumberField({
  id,
  value,
  min,
  max,
  onCommit,
  suffix,
  testId,
}: {
  id?: string
  value: number
  min: number
  max: number
  onCommit: (next: number) => void
  suffix?: string
  testId?: string
}) {
  const { t } = useTranslation()
  const errorId = useId()
  const [draft, setDraft] = useState<string | null>(null)
  const text = draft ?? String(value)
  const parsed = Number(text)
  const invalid =
    draft !== null &&
    (text.trim() === "" ||
      !Number.isInteger(parsed) ||
      parsed < min ||
      parsed > max)

  function commit() {
    if (draft === null || invalid) return
    setDraft(null)
    if (parsed !== value) onCommit(parsed)
  }

  return (
    <div className="flex flex-col gap-1">
      <div className="flex items-center gap-2">
        <Input
          id={id}
          type="number"
          inputMode="numeric"
          min={min}
          max={max}
          step={1}
          value={text}
          onChange={(e) => setDraft(e.target.value)}
          onBlur={commit}
          onKeyDown={(e) => {
            if (e.key === "Enter") commit()
          }}
          aria-invalid={invalid || undefined}
          aria-describedby={invalid ? errorId : undefined}
          className="w-24 tabular-nums pointer-coarse:h-11"
          data-testid={testId}
        />
        {suffix && <span className="text-body text-ink-muted">{suffix}</span>}
      </div>
      {invalid && (
        <p id={errorId} className="text-small text-danger">
          {t("settings_page.number_range", { min, max })}
        </p>
      )}
    </div>
  )
}

export interface OptionCard<V extends string> {
  value: V
  label: string
  detail?: React.ReactNode
}

/** A radio group drawn as a row of cards. */
export function OptionCards<V extends string>({
  name,
  legend,
  options,
  value,
  onChange,
  columns = 4,
}: {
  name: string
  legend: string
  options: OptionCard<V>[]
  value: V | null
  onChange: (next: V) => void
  columns?: 2 | 3 | 4
}) {
  return (
    <fieldset>
      <legend className="sr-only">{legend}</legend>
      <div
        className={cn(
          "grid gap-2",
          columns === 4 && "grid-cols-2 lg:grid-cols-4",
          columns === 3 && "grid-cols-3",
          columns === 2 && "grid-cols-2"
        )}
      >
        {options.map((opt) => (
          <label
            key={opt.value}
            className={cn(
              "relative flex min-h-11 cursor-pointer flex-col justify-center gap-0.5 rounded-lg border px-3 py-2 transition-colors duration-150 ease-out",
              "has-[:focus-visible]:outline-2 has-[:focus-visible]:outline-offset-2 has-[:focus-visible]:outline-accent",
              value === opt.value
                ? "border-accent bg-accent/10 text-ink"
                : "border-line bg-panel text-ink hover:border-ink-muted"
            )}
            data-testid={`option-${name}-${opt.value}`}
          >
            <input
              type="radio"
              name={name}
              value={opt.value}
              checked={value === opt.value}
              onChange={() => onChange(opt.value)}
              className="sr-only"
            />
            <span className="text-body font-semibold">{opt.label}</span>
            {opt.detail && (
              <span className="flex flex-col text-small text-ink-muted tabular-nums">
                {opt.detail}
              </span>
            )}
          </label>
        ))}
      </div>
    </fieldset>
  )
}

/** A titled group of setting rows inside a section. */
export function SettingsGroup({
  title,
  description,
  saved,
  children,
}: {
  title?: string
  /** The group's effect line, for a group holding one wide control. */
  description?: React.ReactNode
  /** Shows the "Saved" note by the title (pass a boolean to enable it). */
  saved?: boolean
  children: React.ReactNode
}) {
  return (
    <section className="flex flex-col">
      {title && (
        <div className="flex flex-col gap-1 border-b border-line pb-2">
          <div className="flex flex-wrap items-baseline gap-2">
            <h3 className="text-h3 font-semibold text-ink">{title}</h3>
            {saved !== undefined && <SavedNote saved={saved} />}
          </div>
          {description && (
            <p className="text-small text-ink-muted">{description}</p>
          )}
        </div>
      )}
      <div className="flex flex-col">{children}</div>
    </section>
  )
}

/** The quiet "Saved" note. The live region stays mounted so screen readers
 *  announce it when it appears. */
export function SavedNote({ saved }: { saved: boolean }) {
  const { t } = useTranslation()
  return (
    <span
      role="status"
      className="inline-flex items-center gap-1 text-small text-ink-muted"
    >
      {saved && (
        <>
          <Check className="size-4" strokeWidth={1.5} aria-hidden />
          {t("settings_page.saved")}
        </>
      )}
    </span>
  )
}
