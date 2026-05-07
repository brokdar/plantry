import { format, parseISO } from "date-fns"
import { de as deLocale } from "date-fns/locale"
import { Bookmark, CalendarDays, CalendarRange } from "lucide-react"
import { useMemo, useState } from "react"
import { useTranslation } from "react-i18next"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Skeleton } from "@/components/ui/skeleton"
import type {
  ApplyConflict,
  ApplyTemplateInput,
  Template,
} from "@/lib/api/templates"
import { useApplyTemplate, useTemplates } from "@/lib/queries/templates"
import { useTimeSlots } from "@/lib/queries/slots"
import { slotLabel } from "@/lib/slot-label"
import { toast, toastError } from "@/lib/toast"
import { cn } from "@/lib/utils"

export type TemplatePickerScope = "day" | "week"

export interface TemplatePickerOverlap {
  /** Set of "date|slotId" keys for slots already filled in the impacted range.
   * The picker uses this set to count overlaps for the chosen template's
   * target dates and to gate the conflict UI. */
  occupied: Set<string>
}

export interface TemplatePickerApplyInfo {
  templateName: string
  scope: TemplatePickerScope
  /** Targets the user picked, after dedup. */
  resolvedTargets: { date: string; slot_id: number }[]
  /** Backend returned new plates. */
  created: { id: number; date: string; slot_id: number }[]
  /** Backend skipped these (occupied + conflict=skip). */
  skipped: { date: string; slot_id: number }[]
  /** When conflict=overwrite, the (date,slot) keys whose previous plates were
   * deleted server-side. The parent can use this to surface an undo affordance
   * since it has the pre-apply plate snapshot in cache. */
  overwrittenKeys: string[]
}

interface TemplatePickerProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  scope: TemplatePickerScope
  /** Default target date for "day" (YYYY-MM-DD) or week-start for "week". */
  defaultDate: string
  /** Lookup of already-occupied slots so the picker can warn before apply. */
  overlap?: TemplatePickerOverlap
  /** Called synchronously before the apply mutation runs, with the keys that
   * will be overwritten if conflict=overwrite. Lets the parent snapshot
   * pre-apply state from cache while it's still fresh. */
  onBeforeApply?: (info: { overwrittenKeys: string[] }) => void
  /** Optional hook for parent-owned post-apply behavior (e.g. undo toast on
   * overwrite). When provided, the picker does not fire its own success toast. */
  onApplied?: (info: TemplatePickerApplyInfo) => void
}

const SCOPE_ICONS = {
  day: CalendarDays,
  week: CalendarRange,
} as const

export function TemplatePicker(props: TemplatePickerProps) {
  // The body owns transient picker state. Remounting it whenever the dialog
  // opens (via a fresh key per open) resets selection, date, and conflict
  // without needing a setState-in-effect to clear leftovers.
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      {props.open && (
        <TemplatePickerBody
          key={`${props.scope}:${props.defaultDate}`}
          {...props}
        />
      )}
    </Dialog>
  )
}

function TemplatePickerBody({
  onOpenChange,
  scope,
  defaultDate,
  overlap,
  onBeforeApply,
  onApplied,
}: TemplatePickerProps) {
  const { t, i18n } = useTranslation()
  const Icon = SCOPE_ICONS[scope]
  const templatesQuery = useTemplates(scope)
  const slotsQuery = useTimeSlots(true)
  const applyMut = useApplyTemplate()

  const slotsById = useMemo(() => {
    const map = new Map<number, { id: number; name_key: string }>()
    for (const s of slotsQuery.data?.items ?? []) map.set(s.id, s)
    return map
  }, [slotsQuery.data])

  const [selectedId, setSelectedId] = useState<number | null>(null)
  const [date, setDate] = useState(defaultDate)
  const [conflict, setConflict] = useState<ApplyConflict>("skip")

  const templates = useMemo(
    () => templatesQuery.data ?? [],
    [templatesQuery.data]
  )
  const selected = useMemo(
    () => templates.find((t) => t.id === selectedId) ?? null,
    [templates, selectedId]
  )

  const overlapCount = useMemo(() => {
    if (!selected || !overlap) return 0
    return countOverlaps(selected, date, scope, overlap.occupied)
  }, [selected, overlap, date, scope])

  const resolvedTargets = useMemo(
    () => (selected ? resolveTargets(selected, date, scope) : []),
    [selected, date, scope]
  )

  const dateLocale = i18n.language?.startsWith("de") ? deLocale : undefined
  const dateLabel = (d: string) =>
    format(parseISO(d), "EEE, MMM d", { locale: dateLocale })

  const titleKey =
    scope === "day" ? "template.apply_day_title" : "template.apply_week_title"
  const targetKey =
    scope === "day" ? "template.apply_day_target" : "template.apply_week_target"
  const emptyKey =
    scope === "day"
      ? "template.apply_picker_empty_day"
      : "template.apply_picker_empty_week"

  async function handleSubmit() {
    if (!selected) return
    const input: ApplyTemplateInput =
      scope === "day" ? { date, conflict } : { start_date: date, conflict }
    // Compute the keys that *would* be overwritten *before* the mutation —
    // after the apply lands, the cache has already moved on. Overwrite only
    // happens when conflict=overwrite AND the target is currently occupied
    // (i.e. server skipped count would have been > 0 in skip mode).
    const overwrittenKeys =
      conflict === "overwrite" && overlap
        ? resolvedTargets
            .filter((tgt) => overlap.occupied.has(`${tgt.date}|${tgt.slot_id}`))
            .map((tgt) => `${tgt.date}|${tgt.slot_id}`)
        : []
    onBeforeApply?.({ overwrittenKeys })
    try {
      const result = await applyMut.mutateAsync({
        templateId: selected.id,
        input,
      })
      const createdList = result.plates ?? []
      const skippedList = result.skipped ?? []
      if (onApplied) {
        onApplied({
          templateName: selected.name,
          scope,
          resolvedTargets,
          created: createdList,
          skipped: skippedList,
          overwrittenKeys,
        })
      } else {
        if (createdList.length > 0) {
          toast.success(
            t("template.apply_result", { count: createdList.length })
          )
        }
        if (skippedList.length > 0) {
          toast(
            t("template.apply_result_skipped", { count: skippedList.length })
          )
        }
      }
      onOpenChange(false)
    } catch (err) {
      toastError(err, t)
    }
  }

  return (
    <DialogContent className="max-w-md">
      <DialogHeader>
        <DialogTitle className="flex items-center gap-2">
          <Icon className="size-4" aria-hidden />
          {t(titleKey)}
        </DialogTitle>
        <DialogDescription>
          {t(`template.scope.${scope}_section_title`)}
        </DialogDescription>
      </DialogHeader>

      <div className="space-y-4">
        {/* Target date input */}
        <div className="space-y-1.5">
          <Label htmlFor="template-picker-date" className="text-xs">
            {t(targetKey)}
          </Label>
          <Input
            id="template-picker-date"
            type="date"
            value={date}
            onChange={(e) => setDate(e.target.value)}
            data-testid="template-picker-date"
          />
        </div>

        {/* Template list */}
        {templatesQuery.isLoading ? (
          <div className="space-y-2">
            <Skeleton className="h-10 w-full" />
            <Skeleton className="h-10 w-full" />
          </div>
        ) : templates.length === 0 ? (
          <p
            className="rounded-md border border-dashed border-outline-variant/50 bg-surface-container-low/40 px-3 py-6 text-center text-sm text-on-surface-variant"
            data-testid="template-picker-empty"
          >
            {t(emptyKey)}
          </p>
        ) : (
          <ul
            className="max-h-64 space-y-1.5 overflow-y-auto pr-1"
            role="list"
            data-testid="template-picker-list"
          >
            {templates.map((tpl) => (
              <li key={tpl.id}>
                <button
                  type="button"
                  onClick={() => setSelectedId(tpl.id)}
                  data-testid={`template-picker-item-${tpl.id}`}
                  aria-pressed={selectedId === tpl.id}
                  className={cn(
                    "group flex w-full items-center justify-between gap-3 rounded-md border bg-surface-container-lowest px-3 py-2 text-left transition-colors",
                    selectedId === tpl.id
                      ? "border-primary/60 bg-primary/8"
                      : "border-outline-variant/40 hover:border-primary/40 hover:bg-surface-container-low"
                  )}
                >
                  <span className="flex min-w-0 items-center gap-2.5">
                    <Bookmark
                      className="size-3.5 shrink-0 text-secondary"
                      aria-hidden
                    />
                    <span className="truncate text-sm font-medium text-on-surface">
                      {tpl.name}
                    </span>
                  </span>
                  <Badge variant="secondary" className="shrink-0 text-xs">
                    {t("template.plates_count", {
                      count: countPlates(tpl),
                    })}
                  </Badge>
                </button>
              </li>
            ))}
          </ul>
        )}

        {/* Preview the resolved (date, slot) pairs that will be written.
          Trust before destructive action: users see exactly what lands. */}
        {selected && resolvedTargets.length > 0 && (
          <div
            className="space-y-1.5 rounded-md border border-outline-variant/40 bg-surface-container-low/40 px-3 py-2"
            data-testid="template-picker-preview"
          >
            <p className="font-heading text-[10px] font-bold tracking-[0.18em] text-on-surface-variant uppercase">
              {t("template.apply_picker_preview_label")}
            </p>
            <ul className="flex flex-wrap gap-1">
              {resolvedTargets.slice(0, 6).map((tgt) => {
                const slot = slotsById.get(tgt.slot_id)
                const slotName = slot
                  ? slotLabel(t, slot.name_key)
                  : `#${tgt.slot_id}`
                return (
                  <li
                    key={`${tgt.date}|${tgt.slot_id}`}
                    className="rounded bg-surface-container-lowest px-1.5 py-0.5 font-mono text-[11px] text-on-surface"
                  >
                    {scope === "week"
                      ? `${dateLabel(tgt.date)} · ${slotName}`
                      : slotName}
                  </li>
                )
              })}
              {resolvedTargets.length > 6 && (
                <li className="self-center px-1.5 py-0.5 text-[11px] text-on-surface-variant">
                  {t("template.apply_picker_preview_more", {
                    count: resolvedTargets.length - 6,
                  })}
                </li>
              )}
            </ul>
          </div>
        )}

        {/* Conflict resolution — only if any overlap detected */}
        {selected && overlapCount > 0 && (
          <fieldset
            className="space-y-2 rounded-md border border-amber-300/40 bg-amber-50/40 px-3 py-3 dark:border-amber-500/30 dark:bg-amber-500/8"
            data-testid="template-picker-conflict"
          >
            <legend className="px-1 text-xs font-medium text-on-surface">
              {t("template.apply_conflict_label")}
            </legend>
            <p className="text-xs text-on-surface-variant">
              {t("template.apply_overlap_warning", { count: overlapCount })}{" "}
              {scope === "day" && date ? `(${dateLabel(date)})` : null}
            </p>
            <div className="space-y-1.5">
              <ConflictOption
                value="skip"
                selected={conflict}
                onChange={setConflict}
                label={t("template.apply_conflict_skip")}
              />
              <ConflictOption
                value="overwrite"
                selected={conflict}
                onChange={setConflict}
                label={t("template.apply_conflict_overwrite")}
              />
            </div>
          </fieldset>
        )}
      </div>

      <DialogFooter>
        <Button
          type="button"
          variant="outline"
          onClick={() => onOpenChange(false)}
        >
          {t("common.cancel")}
        </Button>
        <Button
          type="button"
          onClick={() => void handleSubmit()}
          disabled={!selected || !date || applyMut.isPending}
          data-testid="template-picker-submit"
        >
          {t("template.apply_submit")}
        </Button>
      </DialogFooter>
    </DialogContent>
  )
}

function ConflictOption({
  value,
  selected,
  onChange,
  label,
}: {
  value: ApplyConflict
  selected: ApplyConflict
  onChange: (v: ApplyConflict) => void
  label: string
}) {
  const id = `conflict-${value}`
  return (
    <label
      htmlFor={id}
      className="flex cursor-pointer items-center gap-2 text-sm text-on-surface"
    >
      <input
        id={id}
        type="radio"
        name="template-picker-conflict"
        value={value}
        checked={selected === value}
        onChange={() => onChange(value)}
        className="size-3.5 accent-primary"
      />
      {label}
    </label>
  )
}

/** Counts how many of the template's target (date, slot) pairs land on a slot
 * already in the `occupied` set. Slot-scope templates aren't pickable here,
 * so day/week is the only path. */
function countOverlaps(
  tpl: Template,
  date: string,
  scope: TemplatePickerScope,
  occupied: Set<string>
): number {
  const targets = resolveTargets(tpl, date, scope)
  let count = 0
  for (const tgt of targets) {
    if (occupied.has(`${tgt.date}|${tgt.slot_id}`)) count++
  }
  return count
}

/** Resolves the unique (date, slot) targets a template will write when
 * applied at `date`. Day-scope flattens to the same date for every entry;
 * week-scope offsets each entry by its day_offset. Multiple components on
 * the same plate collapse to one target. */
function resolveTargets(
  tpl: Template,
  date: string,
  scope: TemplatePickerScope
): { date: string; slot_id: number }[] {
  if (!date) return []
  const start = parseISO(date)
  const seen = new Set<string>()
  const out: { date: string; slot_id: number }[] = []
  for (const e of tpl.components) {
    if (e.slot_id == null) continue
    const offset = scope === "week" ? e.day_offset : 0
    const target = new Date(start)
    target.setDate(start.getDate() + offset)
    const dateStr = `${target.getFullYear()}-${String(target.getMonth() + 1).padStart(2, "0")}-${String(target.getDate()).padStart(2, "0")}`
    const key = `${dateStr}|${e.slot_id}`
    if (seen.has(key)) continue
    seen.add(key)
    out.push({ date: dateStr, slot_id: e.slot_id })
  }
  // Sort by (date, slot_id) for stable preview ordering.
  out.sort((a, b) =>
    a.date < b.date ? -1 : a.date > b.date ? 1 : a.slot_id - b.slot_id
  )
  return out
}

/** Plate count for a day/week template = unique (day_offset, slot_id) pairs.
 * Each plate may contain multiple components; the badge counts plates, not
 * components, so the user's mental model matches. */
function countPlates(tpl: Template): number {
  const seen = new Set<string>()
  for (const e of tpl.components) {
    if (e.slot_id == null) continue
    seen.add(`${e.day_offset}|${e.slot_id}`)
  }
  return seen.size
}
