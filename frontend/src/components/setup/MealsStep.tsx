import { ArrowDown, ArrowUp } from "lucide-react"
import { useMemo } from "react"
import { useTranslation } from "react-i18next"

import { AvailabilityGrid } from "@/components/availability/AvailabilityGrid"
import { Switch } from "@/components/settings/controls"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { weekdayColumns } from "@/lib/availability-grid"
import { usePresets } from "@/lib/queries/presets"
import { SLOT_ICONS, SLOT_ICON_FALLBACK } from "@/lib/slot-icons"

import { SetupField } from "./SetupField"
import { slotDraftName, type MealsDraft, type SlotDraft } from "./wizard-draft"

const PRESET_PARAMS = { sort: "name", limit: 200 } as const

/** Plan D15: the meal slots (rename, reorder, on/off) and the usual week
 *  over the slots that are on. Nothing is saved until Next. */
export function MealsStep({
  value,
  onChange,
}: {
  value: MealsDraft
  onChange: (next: MealsDraft) => void
}) {
  const { t } = useTranslation()
  const presetsQuery = usePresets(PRESET_PARAMS)
  const presets = useMemo(
    () =>
      (presetsQuery.data?.items ?? []).map((p) => ({ id: p.id, name: p.name })),
    [presetsQuery.data]
  )
  const { slots, grid } = value

  function setSlot(index: number, patch: Partial<SlotDraft>) {
    onChange({
      ...value,
      slots: slots.map((s, i) => (i === index ? { ...s, ...patch } : s)),
    })
  }

  function move(index: number, by: -1 | 1) {
    const next = [...slots]
    const [slot] = next.splice(index, 1)
    next.splice(index + by, 0, slot!)
    onChange({ ...value, slots: next })
  }

  const gridSlots = slots
    .filter((s) => s.active)
    .map((s) => ({ id: s.id, label: slotDraftName(t, s).trim() || "—" }))
  const gridLabel = t("settings_page.meals.grid_label")

  return (
    <div className="flex flex-col">
      <SetupField
        label={t("settings_page.meals.slots_label")}
        description={t("settings_page.meals.slots_effect")}
      >
        <ul className="flex flex-col divide-y divide-line rounded-lg border border-line bg-panel">
          {slots.map((slot, i) => {
            const Icon = SLOT_ICONS[slot.icon] ?? SLOT_ICON_FALLBACK
            const shown = slotDraftName(t, slot)
            const name = shown.trim() || t("setup.meals.unnamed")
            return (
              <li
                key={slot.id}
                className="flex items-center gap-2 px-3 py-2"
                data-testid={`setup-slot-${i}`}
              >
                <Icon
                  className="size-4 shrink-0 text-ink-muted"
                  strokeWidth={1.5}
                  aria-hidden
                />
                <Input
                  value={shown}
                  onChange={(e) => setSlot(i, { label: e.target.value })}
                  aria-label={t("setup.meals.name_label", { n: i + 1 })}
                  aria-invalid={shown.trim() === "" || undefined}
                  className="min-w-0 flex-1 pointer-coarse:h-11"
                />
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="pointer-coarse:size-11"
                  aria-label={t("setup.meals.move_up", { name })}
                  disabled={i === 0}
                  onClick={() => move(i, -1)}
                >
                  <ArrowUp strokeWidth={1.5} aria-hidden />
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="pointer-coarse:size-11"
                  aria-label={t("setup.meals.move_down", { name })}
                  disabled={i === slots.length - 1}
                  onClick={() => move(i, 1)}
                >
                  <ArrowDown strokeWidth={1.5} aria-hidden />
                </Button>
                <Switch
                  checked={slot.active}
                  onChange={(active) => setSlot(i, { active })}
                  label={t("setup.meals.plan_toggle", { name })}
                />
              </li>
            )
          })}
        </ul>
      </SetupField>

      <SetupField
        label={gridLabel}
        description={t("settings_page.meals.grid_effect")}
      >
        {gridSlots.length === 0 ? (
          <p className="text-body text-ink-muted">
            {t("setup.meals.none_active")}
          </p>
        ) : (
          <AvailabilityGrid
            label={gridLabel}
            columns={weekdayColumns(t)}
            slots={gridSlots}
            value={grid}
            presets={presets}
            surface="ground"
            onChange={(colKey, slotId, cell) =>
              onChange({
                ...value,
                grid: { ...grid, [`${colKey}:${slotId}`]: cell },
              })
            }
            onBulkChange={(patch) =>
              onChange({ ...value, grid: { ...grid, ...patch } })
            }
          />
        )}
      </SetupField>
    </div>
  )
}
