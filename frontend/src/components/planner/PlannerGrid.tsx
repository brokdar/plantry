import {
  addDays,
  isSameDay,
  setISOWeek,
  setISOWeekYear,
  startOfISOWeek,
} from "date-fns"
import {
  DndContext,
  KeyboardSensor,
  PointerSensor,
  closestCorners,
  useSensor,
  useSensors,
  type DragEndEvent,
  type DragStartEvent,
} from "@dnd-kit/core"
import { useNavigate } from "@tanstack/react-router"
import * as Lucide from "lucide-react"
import { useMemo, useRef, useState } from "react"
import { useTranslation } from "react-i18next"

import { SaveAsTemplateDialog } from "@/components/templates/SaveAsTemplateDialog"
import {
  dragStartToPayload,
  DndCellWrapper,
  type DragPayload,
} from "@/components/planner/DndCellWrapper"
import { addPlateComponent, updatePlate } from "@/lib/api/plates"
import { createPlate } from "@/lib/api/weeks"
import { queryClient } from "@/lib/query-client"
import { weekKeys } from "@/lib/queries/keys"
import type { Component } from "@/lib/api/components"
import type { TimeSlot } from "@/lib/api/slots"
import type { Template } from "@/lib/api/templates"
import type { Week } from "@/lib/api/weeks"
import {
  useComponents,
  useSetComponentFavorite,
} from "@/lib/queries/components"
import { useClearFeedback, useRecordFeedback } from "@/lib/queries/feedback"
import { findPlateAt } from "@/lib/queries/plate-patches"
import {
  useAddPlateComponent,
  useDeletePlate,
  useSetPlateSkipped,
  useSwapPlateComponent,
} from "@/lib/queries/plates"
import { useApplyTemplate } from "@/lib/queries/templates"
import { useWeekNutrition, useCreatePlate } from "@/lib/queries/weeks"
import { slotLabel } from "@/lib/slot-label"
import { usePlannerUI } from "@/lib/stores/planner-ui"
import { toastError } from "@/lib/toast"

import { AddComponentSheet } from "./AddComponentSheet"
import { DayHeader } from "./DayHeader"
import { SlotCell } from "./SlotCell"

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
  const navigate = useNavigate()

  const openPicker = (day: number, slotId: number) =>
    void navigate({
      to: "/planner/$weekId/$day/$slotId/pick",
      params: {
        weekId: String(week.id),
        day: String(day),
        slotId: String(slotId),
      },
    })
  const componentsQuery = useComponents({ limit: 200 })
  const componentsById = useMemo(() => {
    const map = new Map<number, Component>()
    for (const c of componentsQuery.data?.items ?? []) map.set(c.id, c)
    return map
  }, [componentsQuery.data])

  const weekStart = useMemo(() => {
    const d = setISOWeekYear(new Date(), week.year)
    return startOfISOWeek(setISOWeek(d, week.week_number))
  }, [week.year, week.week_number])

  const today = new Date()

  const nutritionQuery = useWeekNutrition(week.id)
  const dayMacros = useMemo(() => {
    const m = new Map<
      number,
      NonNullable<typeof nutritionQuery.data>["days"][number]["macros"]
    >()
    for (const d of nutritionQuery.data?.days ?? []) m.set(d.day, d.macros)
    return m
  }, [nutritionQuery, nutritionQuery.data])

  const [addTarget, setAddTarget] = useState<AddTarget | null>(null)
  const [swapTarget, setSwapTarget] = useState<SwapTarget | null>(null)
  const [savePlateId, setSavePlateId] = useState<number | null>(null)

  const createPlateMut = useCreatePlate(week.id)
  const addCompMut = useAddPlateComponent(week.id)
  const swapMut = useSwapPlateComponent(week.id)
  const deletePlateMut = useDeletePlate(week.id)
  const setSkippedMut = useSetPlateSkipped(week.id)
  const applyTemplateMut = useApplyTemplate(week.id)
  const setFavoriteMut = useSetComponentFavorite()
  const recordFeedbackMut = useRecordFeedback(week.id)
  const clearFeedbackMut = useClearFeedback(week.id)

  const aiFill = usePlannerUI((s) => s.aiFill)
  const clearAiFillOnPlate = usePlannerUI((s) => s.clearAiFillOnPlate)
  const aiFilledIds = useMemo(() => {
    if (!aiFill || aiFill.weekId !== week.id) return new Set<number>()
    return new Set(aiFill.aiFilledPlateIds)
  }, [aiFill, week.id])

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

  async function handleDeletePlate(plateId: number) {
    if (!window.confirm(t("plate.delete_confirm_body"))) return
    try {
      await deletePlateMut.mutateAsync(plateId)
    } catch (err) {
      toastError(err, t)
    }
  }

  async function handleToggleSkip(
    day: number,
    slotId: number,
    plateId: number | null
  ) {
    try {
      // Create a plate first if none exists at the target slot, then flip the
      // skip flag atomically on the backend. Skipping clears components.
      let id = plateId
      if (id === null) {
        const created = await createPlateMut.mutateAsync({
          day,
          slot_id: slotId,
        })
        id = created.id
      }
      const existing = findPlateAt(week, day, slotId)
      const nextSkipped = !existing?.skipped
      await setSkippedMut.mutateAsync({
        plateId: id,
        input: { skipped: nextSkipped, note: existing?.note ?? null },
      })
    } catch (err) {
      toastError(err, t)
    }
  }

  async function handleToggleFavorite(
    componentId: number | undefined,
    current: boolean
  ) {
    if (!componentId) return
    try {
      await setFavoriteMut.mutateAsync({ id: componentId, favorite: !current })
    } catch (err) {
      toastError(err, t)
    }
  }

  async function handleRate(
    plateId: number,
    status: "loved" | "disliked",
    current?: string
  ) {
    try {
      if (current === status) {
        await clearFeedbackMut.mutateAsync(plateId)
      } else {
        await recordFeedbackMut.mutateAsync({ plateId, input: { status } })
      }
    } catch (err) {
      toastError(err, t)
    }
  }

  // Drag-and-drop: a plain drag is "move", holding ⌘ (macOS) or Ctrl (others)
  // during drag-start switches to "copy". Because dnd-kit doesn't expose the
  // modifier key on drag events, we stash it from the activator event into a
  // ref that onDragEnd reads.
  const headerScrollRef = useRef<HTMLDivElement>(null)
  const bodyScrollRef = useRef<HTMLDivElement>(null)
  const modeRef = useRef<"move" | "copy">("move")
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 6 } }),
    useSensor(KeyboardSensor)
  )

  function handleDragStart(event: DragStartEvent) {
    const activator = event.activatorEvent as
      | MouseEvent
      | KeyboardEvent
      | undefined
    const copyHeld =
      !!activator &&
      "metaKey" in activator &&
      (activator.metaKey || activator.ctrlKey)
    modeRef.current = copyHeld ? "copy" : "move"
    // Keep the payload helper warm for future use (e.g. custom preview).
    const payload: DragPayload | null = dragStartToPayload(event, !!copyHeld)
    if (!payload) modeRef.current = "move"
  }

  async function handleDragEnd(event: DragEndEvent) {
    const active = event.active
    const over = event.over
    if (!over) return
    const activeData = active.data.current as
      | { plateId?: number; day?: number; slotId?: number }
      | undefined
    const overData = over.data.current as
      | {
          day?: number
          slotId?: number
          existingPlateId?: number
          skipped?: boolean
        }
      | undefined
    if (!activeData?.plateId || !overData) return
    if (overData.skipped) {
      toastError(
        new Error(
          t("planner.dnd.reject_skipped", {
            defaultValue: "Slot is marked skip",
          })
        ),
        t
      )
      return
    }
    if (
      activeData.day === overData.day &&
      activeData.slotId === overData.slotId
    ) {
      return
    }

    try {
      if (modeRef.current === "move") {
        await updatePlate(activeData.plateId, {
          day: overData.day,
          slot_id: overData.slotId,
        })
      } else {
        const src = findPlateAt(week, activeData.day!, activeData.slotId!)
        if (!src) return
        const created = await createPlate(week.id, {
          day: overData.day!,
          slot_id: overData.slotId!,
          note: src.note,
        })
        for (const pc of src.components) {
          await addPlateComponent(created.id, {
            component_id: pc.component_id,
            portions: pc.portions,
          })
        }
      }
      await queryClient.invalidateQueries({ queryKey: weekKeys.all })
    } catch (err) {
      toastError(err, t)
    }
  }

  return (
    <DndContext
      sensors={sensors}
      collisionDetection={closestCorners}
      onDragStart={handleDragStart}
      onDragEnd={handleDragEnd}
    >
      <div>
        {/* Sticky header row — lives outside overflow-x-auto so page-scroll sticky works */}
        <div className="sticky top-16 z-20">
          <div ref={headerScrollRef} className="hide-scrollbar overflow-x-auto">
            <div
              className="grid min-w-[960px] gap-2.5 rounded-t-3xl border-t border-r border-l border-outline-variant/40 bg-surface-container-lowest px-5 pt-5"
              style={{ gridTemplateColumns: "130px repeat(7, minmax(0, 1fr))" }}
            >
              <div />
              {DAY_KEYS.map((dayKey, idx) => {
                const date = addDays(weekStart, idx)
                const isToday = isSameDay(date, today)
                return (
                  <DayHeader
                    key={dayKey}
                    idx={idx}
                    dayKey={dayKey}
                    date={date}
                    today={isToday}
                    macros={dayMacros.get(idx)}
                  />
                )
              })}
            </div>
          </div>
        </div>

        {/* Grid body — horizontal scroll synced with header */}
        <div
          ref={bodyScrollRef}
          className="hide-scrollbar overflow-x-auto"
          onScroll={(e) => {
            if (headerScrollRef.current)
              headerScrollRef.current.scrollLeft = e.currentTarget.scrollLeft
          }}
        >
          <div className="editorial-shadow min-w-[960px] rounded-b-3xl border-r border-b border-l border-outline-variant/40 bg-surface-container-lowest px-5 pt-2.5 pb-5">
            <div
              className="grid gap-2.5"
              style={{ gridTemplateColumns: "130px repeat(7, minmax(0, 1fr))" }}
            >
              {slots.map((slot) => (
                <div key={slot.id} className="contents">
                  <div
                    className="flex flex-col items-center justify-center gap-1.5 px-3"
                    data-testid={`slot-row-${slot.id}`}
                  >
                    <span className="grid size-6 place-items-center rounded-lg bg-surface-container text-on-surface-variant">
                      <SlotIcon name={slot.icon} />
                    </span>
                    <span className="font-heading text-[12.5px] font-bold tracking-[0.04em] text-on-surface uppercase">
                      {slotLabel(t, slot.name_key)}
                    </span>
                  </div>
                  {DAY_KEYS.map((_, day) => {
                    const plate = findPlateAt(week, day, slot.id)
                    return (
                      <DndCellWrapper
                        key={`${slot.id}-${day}`}
                        day={day}
                        slotId={slot.id}
                        plate={plate}
                      >
                        <SlotCell
                          day={day}
                          slotId={slot.id}
                          plate={plate}
                          componentsById={componentsById}
                          aiFilled={plate ? aiFilledIds.has(plate.id) : false}
                          onAdd={() => openPicker(day, slot.id)}
                          onDeletePlate={() =>
                            plate && handleDeletePlate(plate.id)
                          }
                          onSaveAsTemplate={
                            plate ? () => setSavePlateId(plate.id) : undefined
                          }
                          onToggleFavorite={() => {
                            const hero = plate?.components
                              .slice()
                              .sort((a, b) => a.sort_order - b.sort_order)[0]
                            const heroComp = hero
                              ? componentsById.get(hero.component_id)
                              : undefined
                            void handleToggleFavorite(
                              heroComp?.id,
                              heroComp?.favorite ?? false
                            )
                            if (plate) clearAiFillOnPlate(plate.id)
                          }}
                          onToggleSkip={() => {
                            void handleToggleSkip(
                              day,
                              slot.id,
                              plate?.id ?? null
                            )
                            if (plate) clearAiFillOnPlate(plate.id)
                          }}
                          onRateLoved={() => {
                            if (!plate) return
                            void handleRate(
                              plate.id,
                              "loved",
                              plate.feedback?.status
                            )
                            clearAiFillOnPlate(plate.id)
                          }}
                          onRateDisliked={() => {
                            if (!plate) return
                            void handleRate(
                              plate.id,
                              "disliked",
                              plate.feedback?.status
                            )
                            clearAiFillOnPlate(plate.id)
                          }}
                        />
                      </DndCellWrapper>
                    )
                  })}
                </div>
              ))}
            </div>
          </div>
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
    </DndContext>
  )
}
