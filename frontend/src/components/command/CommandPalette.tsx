import { useNavigate } from "@tanstack/react-router"
import { Command } from "cmdk"
import { CalendarRange, LayoutList, Repeat, Search } from "lucide-react"
import { useId, useMemo, useState } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import type { ApplyConflict, Preset } from "@/lib/api/presets"
import {
  useApplyPreset,
  useApplyResultToasts,
  usePresets,
} from "@/lib/queries/presets"
import { useTimeSlots } from "@/lib/queries/slots"
import { slotLabel } from "@/lib/slot-label"
import { cn } from "@/lib/utils"

import { GO_TO_ENTRIES } from "./palette-entries"

const itemClass =
  "flex min-h-11 cursor-pointer items-center gap-2 rounded-cell px-2 text-body text-ink aria-selected:bg-panel-sunken md:min-h-9"
const groupClass =
  "[&_[cmdk-group-heading]]:px-2 [&_[cmdk-group-heading]]:pt-3 [&_[cmdk-group-heading]]:pb-1 [&_[cmdk-group-heading]]:text-caption [&_[cmdk-group-heading]]:text-ink-muted"

interface CommandPaletteProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onOpenCopyWeek: () => void
  /** Default day for applying a routine. */
  defaultTargetDate: string
}

export function CommandPalette({
  open,
  onOpenChange,
  onOpenCopyWeek,
  defaultTargetDate,
}: CommandPaletteProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()

  function close() {
    onOpenChange(false)
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-xl gap-0 bg-panel p-0 text-ink">
        <DialogTitle className="sr-only">{t("palette.title")}</DialogTitle>
        {/* Mounted per opening, so the search and the picked routine reset. */}
        {open && (
          <PaletteBody
            defaultTargetDate={defaultTargetDate}
            onNavigate={(to) => {
              close()
              void navigate({ to: to as never })
            }}
            onOpenLibrary={() => {
              close()
              void navigate({
                to: "/settings/$section",
                params: { section: "meals" },
              })
            }}
            onOpenCopyWeek={() => {
              close()
              onOpenCopyWeek()
            }}
            onDone={close}
          />
        )}
      </DialogContent>
    </Dialog>
  )
}

function PaletteBody({
  defaultTargetDate,
  onNavigate,
  onOpenLibrary,
  onOpenCopyWeek,
  onDone,
}: {
  defaultTargetDate: string
  onNavigate: (to: string) => void
  onOpenLibrary: () => void
  onOpenCopyWeek: () => void
  onDone: () => void
}) {
  const { t } = useTranslation()
  const [query, setQuery] = useState("")
  const [routine, setRoutine] = useState<Preset | null>(null)
  const presets = usePresets({ search: query || undefined, sort: "recent" })

  if (routine) {
    return (
      <TargetPicker
        routine={routine}
        defaultTargetDate={defaultTargetDate}
        onCancel={() => setRoutine(null)}
        onDone={onDone}
      />
    )
  }

  return (
    <Command label={t("palette.title")} loop>
      <div className="flex items-center gap-2 border-b border-line px-3 py-2">
        <Search
          className="size-4 text-ink-muted"
          strokeWidth={1.5}
          aria-hidden
        />
        <Command.Input
          value={query}
          onValueChange={setQuery}
          placeholder={t("palette.search_placeholder")}
          className="min-h-9 flex-1 bg-transparent text-body text-ink outline-none placeholder:text-ink-muted"
          autoFocus
        />
      </div>
      <Command.List className="max-h-[60vh] overflow-y-auto p-2">
        <Command.Empty className="px-2 py-6 text-center text-small text-ink-muted">
          {t("palette.no_results")}
        </Command.Empty>

        <Command.Group
          heading={t("palette.section_routines")}
          className={groupClass}
        >
          {(presets.data?.items ?? []).map((preset) => (
            <Command.Item
              key={preset.id}
              value={`routine-${preset.id}-${preset.name}`}
              onSelect={() => setRoutine(preset)}
              className={itemClass}
            >
              <Repeat
                className="size-4 text-ink-muted"
                strokeWidth={1.5}
                aria-hidden
              />
              <span className="flex-1 truncate">{preset.name}</span>
              <span className="text-caption text-ink-muted tabular-nums">
                {t("routines.meals_count", { count: preset.plates.length })}
              </span>
            </Command.Item>
          ))}
        </Command.Group>

        <Command.Group
          heading={t("palette.section_actions")}
          className={groupClass}
        >
          <Command.Item
            value="action-copy-week"
            onSelect={onOpenCopyWeek}
            className={itemClass}
          >
            <CalendarRange
              className="size-4 text-ink-muted"
              strokeWidth={1.5}
              aria-hidden
            />
            {t("palette.copy_week")}
          </Command.Item>
          <Command.Item
            value="action-open-library"
            onSelect={onOpenLibrary}
            className={itemClass}
          >
            <LayoutList
              className="size-4 text-ink-muted"
              strokeWidth={1.5}
              aria-hidden
            />
            {t("palette.open_library")}
          </Command.Item>
        </Command.Group>

        <Command.Group heading={t("nav.go_to")} className={groupClass}>
          {GO_TO_ENTRIES.map(({ to, labelKey, icon: Icon }) => (
            <Command.Item
              key={to}
              value={`go-to${to} ${t(labelKey)}`}
              onSelect={() => onNavigate(to)}
              className={itemClass}
            >
              <Icon
                className="size-4 text-ink-muted"
                strokeWidth={1.5}
                aria-hidden
              />
              {t(labelKey)}
            </Command.Item>
          ))}
        </Command.Group>
      </Command.List>
    </Command>
  )
}

/** Apply a routine to a day: Enter keeps planned meals, Shift+Enter
 *  replaces them. The result toast offers Undo. */
function TargetPicker({
  routine,
  defaultTargetDate,
  onCancel,
  onDone,
}: {
  routine: Preset
  defaultTargetDate: string
  onCancel: () => void
  onDone: () => void
}) {
  const { t } = useTranslation()
  const dateId = useId()
  const [date, setDate] = useState(defaultTargetDate)
  const [slotsFilter, setSlotsFilter] = useState<Set<number>>(new Set())
  const slots = useTimeSlots(true)
  const apply = useApplyPreset()
  const showResult = useApplyResultToasts()

  const routineSlotIds = useMemo(
    () => new Set(routine.plates.map((p) => p.slot_id)),
    [routine.plates]
  )
  const routineSlots = (slots.data?.items ?? []).filter((s) =>
    routineSlotIds.has(s.id)
  )

  function submit(conflict: ApplyConflict) {
    if (!date) return
    apply.mutate(
      {
        presetId: routine.id,
        input: {
          target_date: date,
          on_conflict: conflict,
          slot_ids_filter:
            slotsFilter.size > 0 ? Array.from(slotsFilter) : undefined,
        },
      },
      {
        onSuccess: (result) => {
          showResult(result)
          onDone()
        },
      }
    )
  }

  function toggleSlot(id: number) {
    setSlotsFilter((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  return (
    <div
      className="flex flex-col gap-4 p-4"
      data-testid="palette-target-picker"
    >
      <header className="flex flex-col gap-0.5">
        <p className="text-small text-ink-muted">{t("palette.target_title")}</p>
        <h3 className="text-h3 text-ink">{routine.name}</h3>
      </header>

      <div className="flex flex-col gap-1.5">
        <label htmlFor={dateId} className="text-small text-ink-muted">
          {t("routines.apply.target_date")}
        </label>
        <Input
          id={dateId}
          type="date"
          value={date}
          onChange={(e) => setDate(e.target.value)}
          autoFocus
          className="pointer-coarse:h-11"
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault()
              submit(e.shiftKey ? "overwrite" : "skip")
            }
          }}
        />
      </div>

      {routineSlots.length > 1 && (
        <fieldset className="flex flex-col gap-1.5">
          <legend className="mb-1.5 text-small text-ink-muted">
            {t("palette.slot_filter")}
          </legend>
          <div className="flex flex-wrap gap-1.5">
            {routineSlots.map((slot) => {
              const active = slotsFilter.has(slot.id)
              return (
                <button
                  key={slot.id}
                  type="button"
                  onClick={() => toggleSlot(slot.id)}
                  aria-pressed={active}
                  className={cn(
                    "min-h-11 rounded-cell border px-3 text-small transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent md:min-h-8",
                    active
                      ? "border-accent bg-accent text-on-accent"
                      : "border-line text-ink hover:bg-panel-sunken"
                  )}
                >
                  {slotLabel(t, slot.name_key)}
                </button>
              )
            })}
          </div>
        </fieldset>
      )}

      <div className="flex flex-col gap-3 border-t border-line pt-3 sm:flex-row sm:items-center sm:justify-between">
        <span className="text-caption text-ink-muted">{t("palette.hint")}</span>
        <div className="flex flex-wrap gap-2">
          <Button type="button" variant="outline" onClick={onCancel}>
            {t("common.cancel")}
          </Button>
          <Button
            type="button"
            variant="outline"
            onClick={() => submit("overwrite")}
            disabled={apply.isPending || !date}
          >
            {t("routines.apply.submit_overwrite")}
          </Button>
          <Button
            type="button"
            onClick={() => submit("skip")}
            disabled={apply.isPending || !date}
          >
            {t("routines.apply.submit")}
          </Button>
        </div>
      </div>
    </div>
  )
}
