import { useEffect, useMemo, useState } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import type { SlotBand, SlotBandPreset } from "@/lib/api/slots"
import {
  useApplySlotBandPreset,
  useSetSlotBands,
  useSlotBands,
  useTimeSlots,
} from "@/lib/queries/slots"
import { slotLabel } from "@/lib/slot-label"

import { toast } from "@/lib/toast"

type Draft = { slot_id: number; min: string; max: string }

const toDraft = (b: SlotBand): Draft => ({
  slot_id: b.slot_id,
  min: String(b.min_pct),
  max: String(b.max_pct),
})

const num = (v: string) => (v.trim() === "" ? NaN : Number(v))

/** The energy band of every meal slot: the share of the day a meal should hold.
 *  The running sums are shown, and a set whose minimums add up to more than 100
 *  or whose maximums add up to less than 100 cannot be saved. */
export function SlotBandsEditor() {
  const { t } = useTranslation()
  const bandsQuery = useSlotBands()
  const slotsQuery = useTimeSlots(true)
  const save = useSetSlotBands()
  const preset = useApplySlotBandPreset()
  const [draft, setDraft] = useState<Draft[]>([])

  const server = bandsQuery.data?.items
  useEffect(() => {
    if (server) setDraft(server.map(toDraft))
  }, [server])

  const names = useMemo(
    () =>
      new Map(
        (slotsQuery.data?.items ?? []).map((s) => [
          s.id,
          slotLabel(t, s.name_key),
        ])
      ),
    [slotsQuery.data, t]
  )

  const { minSum, maxSum, badRow, shares } = useMemo(() => {
    let lo = 0
    let hi = 0
    let bad: number | null = null
    let mids = 0
    for (const d of draft) {
      const a = num(d.min)
      const b = num(d.max)
      if (!(a >= 0 && b <= 100 && a <= b)) bad ??= d.slot_id
      lo += Number.isFinite(a) ? a : 0
      hi += Number.isFinite(b) ? b : 0
      mids += Number.isFinite(a) && Number.isFinite(b) ? (a + b) / 2 : 0
    }
    const sh = new Map<number, number>()
    for (const d of draft) {
      const mid = (num(d.min) + num(d.max)) / 2
      sh.set(
        d.slot_id,
        mids > 0 && Number.isFinite(mid) ? (mid / mids) * 100 : 0
      )
    }
    return { minSum: lo, maxSum: hi, badRow: bad, shares: sh }
  }, [draft])

  const sumsInvalid = draft.length > 0 && (minSum > 100 || maxSum < 100)
  const invalid = sumsInvalid || badRow !== null
  const changed =
    !!server &&
    draft.some((d, i) => {
      const s = server[i]
      return !s || num(d.min) !== s.min_pct || num(d.max) !== s.max_pct
    })

  if (bandsQuery.isLoading || slotsQuery.isLoading) {
    return <Skeleton className="h-40 w-full" />
  }

  const update = (slotId: number, key: "min" | "max", v: string) =>
    setDraft((rows) =>
      rows.map((r) => (r.slot_id === slotId ? { ...r, [key]: v } : r))
    )

  const onSave = () => {
    if (invalid) return
    save.mutate(
      draft.map((d) => ({
        slot_id: d.slot_id,
        min_pct: num(d.min),
        max_pct: num(d.max),
      })),
      { onSuccess: () => toast.success(t("settings_page.meals.bands_saved")) }
    )
  }

  const presets: { id: SlotBandPreset; label: string }[] = [
    { id: "lunch_biggest", label: t("settings_page.meals.bands_preset_lunch") },
    {
      id: "dinner_biggest",
      label: t("settings_page.meals.bands_preset_dinner"),
    },
  ]

  return (
    <div className="flex flex-col gap-4" data-testid="slot-bands">
      <p className="text-small text-ink-muted">
        {t("settings_page.meals.bands_effect")}
      </p>
      <div className="flex flex-wrap gap-2">
        {presets.map((p) => (
          <Button
            key={p.id}
            type="button"
            variant="outline"
            className="pointer-coarse:h-11"
            disabled={preset.isPending}
            onClick={() => preset.mutate(p.id)}
          >
            {p.label}
          </Button>
        ))}
      </div>
      <ul className="flex flex-col gap-3">
        {draft.map((d) => {
          const name = names.get(d.slot_id) ?? String(d.slot_id)
          return (
            <li
              key={d.slot_id}
              className="grid grid-cols-[1fr_5rem_5rem] items-center gap-3"
            >
              <div className="flex flex-col">
                <span className="text-body text-ink">{name}</span>
                <span className="text-caption text-ink-muted">
                  {t("settings_page.meals.bands_share", {
                    pct: Math.round(shares.get(d.slot_id) ?? 0),
                  })}
                </span>
              </div>
              <Input
                type="number"
                inputMode="decimal"
                min={0}
                max={100}
                value={d.min}
                aria-label={`${name} ${t("settings_page.meals.bands_min")}`}
                onChange={(e) => update(d.slot_id, "min", e.target.value)}
                className="pointer-coarse:h-11"
              />
              <Input
                type="number"
                inputMode="decimal"
                min={0}
                max={100}
                value={d.max}
                aria-label={`${name} ${t("settings_page.meals.bands_max")}`}
                onChange={(e) => update(d.slot_id, "max", e.target.value)}
                className="pointer-coarse:h-11"
              />
            </li>
          )
        })}
      </ul>
      <p
        role={invalid ? "alert" : undefined}
        className={
          invalid ? "text-small text-danger" : "text-small text-ink-muted"
        }
      >
        {badRow !== null
          ? t("settings_page.meals.bands_row_invalid", {
              name: names.get(badRow) ?? String(badRow),
            })
          : sumsInvalid
            ? t("settings_page.meals.bands_invalid", {
                min: minSum,
                max: maxSum,
              })
            : t("settings_page.meals.bands_sum", { min: minSum, max: maxSum })}
      </p>
      <div>
        <Button
          type="button"
          disabled={invalid || !changed || save.isPending}
          onClick={onSave}
          className="pointer-coarse:h-11"
        >
          {t("settings_page.meals.bands_save")}
        </Button>
      </div>
    </div>
  )
}
