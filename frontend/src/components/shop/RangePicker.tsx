import { ChevronDown } from "lucide-react"
import { useState } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"
import type { ShoppingRange } from "@/lib/api/shopping"
import { shiftYMD } from "@/lib/dates"
import { formatDate, type Formatter } from "@/lib/plan/format"
import { cn } from "@/lib/utils"

/** Windows are seven days (D24). */
const WINDOW_DAYS = 7

interface RangePickerProps {
  /** The range the list shows. */
  range: ShoppingRange
  /** The plan window the range starts in (`range.window_*`). */
  window: ShoppingRange
  onChange: (range: ShoppingRange) => void
}

/** "Sat, 31 Oct to Fri, 6 Nov" in the active language. */
function rangeText(range: ShoppingRange, f: Formatter): string {
  const options: Intl.DateTimeFormatOptions = {
    weekday: "short",
    day: "numeric",
    month: "short",
  }
  return f.t("shop.range", {
    from: formatDate(range.from, f, options),
    to: formatDate(range.to, f, options),
  })
}

/** "This window" and "Next window", anchored on the viewed window. */
function rangePresets(window: ShoppingRange) {
  return {
    this_window: window,
    next_window: {
      from: shiftYMD(window.from, WINDOW_DAYS),
      to: shiftYMD(window.to, WINDOW_DAYS),
    },
  } as const
}

/**
 * The page title: the shopping range, tappable. It opens the presets "This
 * window" and "Next window" (anchored on the window the range starts in)
 * and a custom from/to range.
 */
export function RangePicker({ range, window, onChange }: RangePickerProps) {
  const { t, i18n } = useTranslation()
  const [open, setOpen] = useState(false)
  const presets = rangePresets(window)

  function choose(next: ShoppingRange) {
    setOpen(false)
    onChange(next)
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <h1 className="text-h1 text-ink">
        <PopoverTrigger asChild>
          <button
            type="button"
            data-testid="shop-range"
            className="-mx-1 inline-flex min-h-11 items-center gap-2 rounded-lg px-1 text-left transition-colors duration-150 ease-out hover:bg-panel-sunken"
          >
            {rangeText(range, i18n)}
            <ChevronDown
              className="size-5 shrink-0 text-ink-muted"
              strokeWidth={1.5}
              aria-hidden="true"
            />
          </button>
        </PopoverTrigger>
      </h1>
      <PopoverContent align="start" className="flex w-80 flex-col gap-4">
        <p className="text-small font-semibold text-ink">
          {t("shop.range_title")}
        </p>
        <div className="flex flex-col gap-1">
          {(Object.keys(presets) as (keyof typeof presets)[]).map((key) => {
            const preset = presets[key]
            const active = preset.from === range.from && preset.to === range.to
            return (
              <button
                key={key}
                type="button"
                aria-pressed={active}
                onClick={() => choose(preset)}
                className={cn(
                  "flex min-h-11 items-center justify-between gap-3 rounded-lg px-3 text-left text-body transition-colors duration-150 ease-out md:pointer-fine:min-h-9",
                  active
                    ? "bg-accent text-on-accent"
                    : "text-ink hover:bg-panel-sunken"
                )}
              >
                <span className="font-semibold">{t(`shop.${key}`)}</span>
                <span
                  className={cn(
                    "text-small tabular-nums",
                    active ? "text-on-accent" : "text-ink-muted"
                  )}
                >
                  {formatDate(preset.from, i18n, {
                    day: "numeric",
                    month: "short",
                  })}
                  {" – "}
                  {formatDate(preset.to, i18n, {
                    day: "numeric",
                    month: "short",
                  })}
                </span>
              </button>
            )
          })}
        </div>
        <CustomRange range={range} onSubmit={choose} />
      </PopoverContent>
    </Popover>
  )
}

/** Two dates and "Show list"; `from` after `to` cannot be submitted. */
function CustomRange({
  range,
  onSubmit,
}: {
  range: ShoppingRange
  onSubmit: (range: ShoppingRange) => void
}) {
  const { t } = useTranslation()
  const [from, setFrom] = useState(range.from)
  const [to, setTo] = useState(range.to)
  const valid = from !== "" && to !== "" && from <= to

  return (
    <form
      className="flex flex-col gap-3 border-t border-line pt-4"
      onSubmit={(e) => {
        e.preventDefault()
        if (valid) onSubmit({ from, to })
      }}
    >
      <p className="text-small font-semibold text-ink">{t("shop.custom")}</p>
      <div className="grid grid-cols-2 gap-3">
        <div className="flex flex-col gap-1">
          <Label htmlFor="shop-range-from">{t("shop.from")}</Label>
          <Input
            id="shop-range-from"
            type="date"
            value={from}
            max={to || undefined}
            onChange={(e) => setFrom(e.target.value)}
            className="h-11 md:pointer-fine:h-9"
          />
        </div>
        <div className="flex flex-col gap-1">
          <Label htmlFor="shop-range-to">{t("shop.to")}</Label>
          <Input
            id="shop-range-to"
            type="date"
            value={to}
            min={from || undefined}
            onChange={(e) => setTo(e.target.value)}
            className="h-11 md:pointer-fine:h-9"
          />
        </div>
      </div>
      <Button
        type="submit"
        disabled={!valid}
        className="h-11 md:pointer-fine:h-9"
      >
        {t("shop.show")}
      </Button>
    </form>
  )
}
