import * as Lucide from "lucide-react"
import { useMemo, useState } from "react"
import { useTranslation } from "react-i18next"

import { SaveAsTemplateDialog } from "@/components/templates/SaveAsTemplateDialog"
import type { Component } from "@/lib/api/components"
import type { TimeSlot } from "@/lib/api/slots"
import type { Template } from "@/lib/api/templates"
import type { Week } from "@/lib/api/weeks"
import { useComponents } from "@/lib/queries/components"
import {
  useAddPlateComponent,
  useDeletePlate,
  useRemovePlateComponent,
  useSwapPlateComponent,
} from "@/lib/queries/plates"
import { findPlateAt } from "@/lib/queries/plate-patches"
import { useApplyTemplate } from "@/lib/queries/templates"
import { useCreatePlate } from "@/lib/queries/weeks"
import { toastError } from "@/lib/toast"

import { cn } from "@/lib/utils"

import { AddComponentSheet } from "./AddComponentSheet"
import { PlateCell } from "./PlateCell"

const DAY_KEYS = [
  "planner.day_mon",
  "planner.day_tue",
  "planner.day_wed",
  "planner.day_thu",
  "planner.day_fri",
  "planner.day_sat",
  "planner.day_sun",
] as const

interface PlannerGridProps {
  week: Week
  slots: TimeSlot[]
}

interface AddTarget {
  day: number
  slotId: number
  plateId: number | null
  defaultRole?: string
}

interface SwapTarget {
  plateId: number
  pcId: number
  defaultRole?: string
}

function SlotIcon({ name }: { name: string }) {
  const Icon = (
    Lucide as unknown as Record<string, Lucide.LucideIcon | undefined>
  )[name]
  if (!Icon) return <Lucide.HelpCircle className="h-4 w-4" aria-hidden />
  return <Icon className="h-4 w-4" aria-hidden />
}

export function PlannerGrid({ week, slots }: PlannerGridProps) {
  const { t } = useTranslation()
  const componentsQuery = useComponents({ limit: 200 })
  const componentsById = useMemo(() => {
    const map = new Map<number, Component>()
    for (const c of componentsQuery.data?.items ?? []) map.set(c.id, c)
    return map
  }, [componentsQuery.data])

  const [addTarget, setAddTarget] = useState<AddTarget | null>(null)
  const [swapTarget, setSwapTarget] = useState<SwapTarget | null>(null)
  const [savePlateId, setSavePlateId] = useState<number | null>(null)

  const createPlateMut = useCreatePlate(week.id)
  const addCompMut = useAddPlateComponent(week.id)
  const swapMut = useSwapPlateComponent(week.id)
  const removeMut = useRemovePlateComponent(week.id)
  const deletePlateMut = useDeletePlate(week.id)
  const applyTemplateMut = useApplyTemplate(week.id)

  async function handlePick(component: Component) {
    if (!addTarget) return
    const target = addTarget
    setAddTarget(null)
    try {
      if (target.plateId === null) {
        await createPlateMut.mutateAsync({
          day: target.day,
          slot_id: target.slotId,
          components: [{ component_id: component.id, portions: 1 }],
        })
      } else {
        await addCompMut.mutateAsync({
          plateId: target.plateId,
          input: { component_id: component.id, portions: 1 },
        })
      }
    } catch (err) {
      toastError(err, t)
    }
  }

  async function handleApplyTemplate(template: Template) {
    if (!addTarget) return
    const target = addTarget
    setAddTarget(null)
    try {
      let plateId = target.plateId
      if (plateId === null) {
        const created = await createPlateMut.mutateAsync({
          day: target.day,
          slot_id: target.slotId,
        })
        plateId = created.id
      }
      await applyTemplateMut.mutateAsync({
        id: template.id,
        input: { plate_id: plateId },
      })
    } catch (err) {
      toastError(err, t)
    }
  }

  async function handleSwapPick(component: Component) {
    if (!swapTarget) return
    const target = swapTarget
    setSwapTarget(null)
    try {
      await swapMut.mutateAsync({
        plateId: target.plateId,
        pcId: target.pcId,
        input: { component_id: component.id },
      })
    } catch (err) {
      toastError(err, t)
    }
  }

  async function handleRemove(plateId: number, pcId: number) {
    try {
      await removeMut.mutateAsync({ plateId, pcId })
    } catch (err) {
      toastError(err, t)
    }
  }

  async function handleDeletePlate(plateId: number) {
    if (!window.confirm(t("plate.delete_confirm_body"))) return
    try {
      await deletePlateMut.mutateAsync(plateId)
    } catch (err) {
      toastError(err, t)
    }
  }

  return (
    <div className="hide-scrollbar overflow-x-auto">
      <div
        className="grid min-w-[900px] gap-0"
        style={{ gridTemplateColumns: `120px repeat(7, minmax(0, 1fr))` }}
      >
        <div className="border-b border-outline-variant/15 px-2 py-3" />
        {DAY_KEYS.map((dayKey, idx) => (
          <div
            key={dayKey}
            className="border-b border-outline-variant/15 px-2 py-3 text-center"
            data-testid={`day-header-${idx}`}
          >
            <p className="font-heading text-xs font-bold tracking-widest text-on-surface-variant uppercase">
              {t(dayKey)}
            </p>
          </div>
        ))}

        {slots.map((slot, rowIdx) => (
          <div
            key={slot.id}
            className={cn(
              "contents",
              rowIdx % 2 === 1 && "[&>*]:bg-surface-container-high/20"
            )}
          >
            <div
              className="flex flex-col items-start justify-center gap-1 border-b border-outline-variant/10 px-3 py-4 text-on-surface-variant"
              data-testid={`slot-row-${slot.id}`}
            >
              <SlotIcon name={slot.icon} />
              <span className="font-heading text-[10px] font-bold tracking-widest uppercase">
                {t(slot.name_key, { defaultValue: slot.name_key })}
              </span>
            </div>
            {DAY_KEYS.map((_, day) => {
              const plate = findPlateAt(week, day, slot.id)
              return (
                <div
                  key={`${slot.id}-${day}`}
                  className="border-b border-outline-variant/10 p-2"
                  data-testid={`cell-${day}-${slot.id}`}
                >
                  <PlateCell
                    plate={plate}
                    weekId={week.id}
                    componentsById={componentsById}
                    onAdd={() =>
                      setAddTarget({
                        day,
                        slotId: slot.id,
                        plateId: plate?.id ?? null,
                        defaultRole: plate ? undefined : "main",
                      })
                    }
                    onSwap={(pcId, role) =>
                      plate &&
                      setSwapTarget({
                        plateId: plate.id,
                        pcId,
                        defaultRole: role,
                      })
                    }
                    onRemoveComponent={(pcId) =>
                      plate && handleRemove(plate.id, pcId)
                    }
                    onDeletePlate={() => plate && handleDeletePlate(plate.id)}
                    onSaveAsTemplate={
                      plate ? () => setSavePlateId(plate.id) : undefined
                    }
                  />
                </div>
              )
            })}
          </div>
        ))}
      </div>

      <AddComponentSheet
        open={addTarget !== null}
        onOpenChange={(o) => !o && setAddTarget(null)}
        defaultRole={addTarget?.defaultRole}
        onPick={handlePick}
        onPickTemplate={handleApplyTemplate}
      />
      <AddComponentSheet
        open={swapTarget !== null}
        onOpenChange={(o) => !o && setSwapTarget(null)}
        defaultRole={swapTarget?.defaultRole}
        onPick={handleSwapPick}
      />
      <SaveAsTemplateDialog
        open={savePlateId !== null}
        onOpenChange={(o) => !o && setSavePlateId(null)}
        plateId={savePlateId}
      />
    </div>
  )
}
