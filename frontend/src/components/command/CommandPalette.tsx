import { useNavigate } from "@tanstack/react-router"
import { Command } from "cmdk"
import { Bookmark, CalendarRange, LayoutList, Search } from "lucide-react"
import { useMemo, useState } from "react"
import { useTranslation } from "react-i18next"

import { Dialog, DialogContent } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Button } from "@/components/ui/button"
import type { ApplySnapshot, Preset } from "@/lib/api/presets"
import { useApplyPreset, usePresets, useUndoApply } from "@/lib/queries/presets"
import { useTimeSlots } from "@/lib/queries/slots"
import { showPresetApplyToasts } from "@/lib/preset-apply-toast"
import { toastError } from "@/lib/toast"

interface CommandPaletteProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onOpenCopyWeek: () => void
  /** Anchor date for the "today" default in the target picker. */
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
  const [query, setQuery] = useState("")
  const [targetForPreset, setTargetForPreset] = useState<Preset | null>(null)
  // Kept at this always-mounted level so the mutation observer survives dialog
  // close — TargetPicker unmounts on close, which would destroy the observer
  // before the toast's Undo action fires.
  const undoMutation = useUndoApply()

  function handleUndo(snapshot: ApplySnapshot) {
    undoMutation.mutate(snapshot, { onError: (err) => toastError(err, t) })
  }

  // Reset query each time the palette opens via re-mount key.
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-xl p-0">
        {open && (
          <PaletteBody
            key={`palette-${open ? "o" : "c"}`}
            t={t}
            navigate={(to: string) => {
              onOpenChange(false)
              navigate({ to: to as never })
            }}
            onOpenCopyWeek={() => {
              onOpenChange(false)
              onOpenCopyWeek()
            }}
            defaultTargetDate={defaultTargetDate}
            close={() => onOpenChange(false)}
            query={query}
            setQuery={setQuery}
            targetForPreset={targetForPreset}
            setTargetForPreset={setTargetForPreset}
            onUndo={handleUndo}
          />
        )}
      </DialogContent>
    </Dialog>
  )
}

interface PaletteBodyProps {
  t: ReturnType<typeof useTranslation>["t"]
  navigate: (to: string) => void
  onOpenCopyWeek: () => void
  defaultTargetDate: string
  close: () => void
  query: string
  setQuery: (q: string) => void
  targetForPreset: Preset | null
  setTargetForPreset: (p: Preset | null) => void
  onUndo: (snapshot: ApplySnapshot) => void
}

function PaletteBody({
  t,
  navigate,
  onOpenCopyWeek,
  defaultTargetDate,
  close,
  query,
  setQuery,
  targetForPreset,
  setTargetForPreset,
  onUndo,
}: PaletteBodyProps) {
  const presetsQuery = usePresets({
    search: query || undefined,
    sort: "recent",
  })

  if (targetForPreset) {
    return (
      <TargetPicker
        preset={targetForPreset}
        defaultTargetDate={defaultTargetDate}
        onCancel={() => setTargetForPreset(null)}
        onDone={close}
        onUndo={onUndo}
      />
    )
  }

  const items = presetsQuery.data?.items ?? []
  return (
    <Command label={t("preset.command.title")} loop>
      <div className="flex items-center gap-2 border-b border-border/40 px-3 py-2">
        <Search className="size-4 text-muted-foreground" aria-hidden />
        <Command.Input
          value={query}
          onValueChange={setQuery}
          placeholder={t("preset.search_placeholder")}
          className="flex-1 bg-transparent text-sm outline-none placeholder:text-muted-foreground"
          autoFocus
        />
      </div>
      <Command.List className="max-h-[60vh] overflow-y-auto p-2">
        <Command.Empty className="px-2 py-6 text-center text-sm text-muted-foreground">
          {t("preset.no_results_title")}
        </Command.Empty>

        <Command.Group heading={t("preset.command.section_presets")}>
          {items.map((preset) => (
            <Command.Item
              key={preset.id}
              value={`preset-${preset.id}-${preset.name}`}
              onSelect={() => setTargetForPreset(preset)}
              className="group flex items-center gap-2 rounded-md px-2 py-1.5 text-sm aria-selected:bg-surface-container-low"
            >
              <Bookmark className="size-3.5 text-muted-foreground" />
              <span className="flex-1 truncate">{preset.name}</span>
              <span className="text-[10px] text-muted-foreground">
                {t("preset.plates_count", { count: preset.plates.length })}
              </span>
            </Command.Item>
          ))}
        </Command.Group>

        <Command.Group heading={t("preset.command.section_actions")}>
          <Command.Item
            value="action-copy-week"
            onSelect={onOpenCopyWeek}
            className="flex items-center gap-2 rounded-md px-2 py-1.5 text-sm aria-selected:bg-surface-container-low"
          >
            <CalendarRange className="size-3.5 text-muted-foreground" />
            {t("preset.command.action_copy_week")}
          </Command.Item>
          <Command.Item
            value="action-open-library"
            onSelect={() => navigate("/presets")}
            className="flex items-center gap-2 rounded-md px-2 py-1.5 text-sm aria-selected:bg-surface-container-low"
          >
            <LayoutList className="size-3.5 text-muted-foreground" />
            {t("preset.command.action_open_library")}
          </Command.Item>
        </Command.Group>
      </Command.List>
      <p className="border-t border-border/40 px-3 py-2 text-[10px] text-muted-foreground">
        {t("preset.command.submit_skip_hint")}
      </p>
    </Command>
  )
}

interface TargetPickerProps {
  preset: Preset
  defaultTargetDate: string
  onCancel: () => void
  onDone: () => void
  onUndo: (snapshot: ApplySnapshot) => void
}

function TargetPicker({
  preset,
  defaultTargetDate,
  onCancel,
  onDone,
  onUndo,
}: TargetPickerProps) {
  const { t } = useTranslation()
  const [date, setDate] = useState(defaultTargetDate)
  const [slotsFilter, setSlotsFilter] = useState<Set<number>>(new Set())
  const slotsQuery = useTimeSlots(true)
  const applyMutation = useApplyPreset()

  const presetSlotIDs = useMemo(
    () => new Set(preset.plates.map((p) => p.slot_id)),
    [preset.plates]
  )
  const activeSlots = (slotsQuery.data?.items ?? []).filter((s) =>
    presetSlotIDs.has(s.id)
  )

  function submit(conflict: "skip" | "overwrite") {
    applyMutation.mutate(
      {
        presetId: preset.id,
        input: {
          target_date: date,
          on_conflict: conflict,
          slot_ids_filter:
            slotsFilter.size > 0 ? Array.from(slotsFilter) : undefined,
        },
      },
      {
        onSuccess: (result) => {
          showPresetApplyToasts(result, t, onUndo)
          onDone()
        },
        onError: (err) => toastError(err, t),
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
    <div className="space-y-4 p-4" data-testid="palette-target-picker">
      <header>
        <p className="font-body text-[10px] font-bold tracking-widest text-muted-foreground uppercase">
          {t("preset.command.target_picker_title")}
        </p>
        <h3 className="font-heading text-lg font-semibold tracking-tight">
          {preset.name}
        </h3>
      </header>

      <label className="flex flex-col gap-1.5 text-sm">
        <span className="font-body text-[11px] font-bold tracking-widest text-muted-foreground uppercase">
          {t("preset.apply.target_date")}
        </span>
        <Input
          type="date"
          value={date}
          onChange={(e) => setDate(e.target.value)}
          autoFocus
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault()
              submit(e.shiftKey ? "overwrite" : "skip")
            }
          }}
        />
      </label>

      {activeSlots.length > 1 && (
        <fieldset className="space-y-1.5">
          <legend className="font-body text-[11px] font-bold tracking-widest text-muted-foreground uppercase">
            {t("preset.command.target_picker_slot_filter")}
          </legend>
          <div className="flex flex-wrap gap-1.5">
            {activeSlots.map((slot) => {
              const active = slotsFilter.has(slot.id)
              return (
                <button
                  key={slot.id}
                  type="button"
                  onClick={() => toggleSlot(slot.id)}
                  aria-pressed={active}
                  className={
                    "rounded-full border px-2.5 py-0.5 text-[11px] transition-colors " +
                    (active
                      ? "border-primary bg-primary/15 text-primary"
                      : "border-border text-muted-foreground hover:border-primary/40")
                  }
                >
                  {t(slot.name_key, { defaultValue: slot.name_key })}
                </button>
              )
            })}
          </div>
        </fieldset>
      )}

      <div className="flex items-center justify-between gap-2 border-t border-border/40 pt-3 text-xs">
        <span className="text-muted-foreground">
          {t("preset.command.submit_skip_hint")}
        </span>
        <div className="flex gap-2">
          <Button type="button" variant="outline" size="sm" onClick={onCancel}>
            {t("common.cancel")}
          </Button>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => submit("overwrite")}
            disabled={applyMutation.isPending}
          >
            {t("preset.apply.submit_overwrite")}
          </Button>
          <Button
            type="button"
            size="sm"
            onClick={() => submit("skip")}
            disabled={applyMutation.isPending}
          >
            {t("preset.apply.submit")}
          </Button>
        </div>
      </div>
    </div>
  )
}
