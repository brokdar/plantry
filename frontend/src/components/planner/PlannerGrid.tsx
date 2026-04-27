import { parseISO, isBefore, isToday, startOfDay } from "date-fns"
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
import * as Lucide from "lucide-react"
import { useMemo, useRef, useState } from "react"
import { useTranslation } from "react-i18next"

import { SaveAsTemplateDialog } from "@/components/templates/SaveAsTemplateDialog"
import {
  dragStartToPayload,
  DndCellWrapper,
  type DragPayload,
} from "@/components/planner/DndCellWrapper"
import { addPlateComponent, createPlate, deletePlate } from "@/lib/api/plates"
import { queryClient } from "@/lib/query-client"
import { plateKeys } from "@/lib/queries/keys"
import type { Food } from "@/lib/api/foods"
import type { Plate } from "@/lib/api/plates"
import type { TimeSlot } from "@/lib/api/slots"
import type { Template } from "@/lib/api/templates"
import { useFoods, useSetFoodFavorite } from "@/lib/queries/foods"
import { useClearFeedback, useRecordFeedback } from "@/lib/queries/feedback"
import {
  useAddPlateComponent,
  useDeletePlate,
  useSetPlateSkipped,
  useSwapPlateComponent,
  useUpdatePlate,
} from "@/lib/queries/plates"
import { useApplyTemplate } from "@/lib/queries/templates"
import { slotLabel } from "@/lib/slot-label"
import { usePlannerUI } from "@/lib/stores/planner-ui"
import { toast, toastError } from "@/lib/toast"

import { AddComponentSheet } from "./AddComponentSheet"
import { DayHeader } from "./DayHeader"
import { SlotCell } from "./SlotCell"

export interface PlannerDay {
  date: string // "YYYY-MM-DD"
  weekday: number // 0=Monday…6=Sunday (matches backend convention)
  plates: Plate[]
}

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
  days: PlannerDay[]
  slots: TimeSlot[]
  rangeFrom: string
  rangeTo: string
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

function findPlateInDay(day: PlannerDay, slotId: number): Plate | undefined {
  return day.plates.find((p) => p.slot_id === slotId)
}

export function PlannerGrid({
  days,
  slots,
  rangeFrom,
  rangeTo,
}: PlannerGridProps) {
  const { t } = useTranslation()

  const openPicker = (day: number, slotId: number) => {
    setAddTarget({ day, slotId, plateId: null })
  }

  const componentsQuery = useFoods({ limit: 200 })
  const componentsById = useMemo(() => {
    const map = new Map<number, Food>()
    for (const c of componentsQuery.data?.items ?? []) map.set(c.id, c)
    return map
  }, [componentsQuery.data])

  const today = startOfDay(new Date())

  const [addTarget, setAddTarget] = useState<AddTarget | null>(null)
  const [swapTarget, setSwapTarget] = useState<SwapTarget | null>(null)
  const [savePlateId, setSavePlateId] = useState<number | null>(null)

  const updatePlateMut = useUpdatePlate(rangeFrom, rangeTo)
  const addCompMut = useAddPlateComponent()
  const swapMut = useSwapPlateComponent()
  const deletePlateMut = useDeletePlate()
  const setSkippedMut = useSetPlateSkipped()
  const applyTemplateMut = useApplyTemplate()
  const setFavoriteMut = useSetFoodFavorite()
  const recordFeedbackMut = useRecordFeedback()
  const clearFeedbackMut = useClearFeedback()

  const aiFill = usePlannerUI((s) => s.aiFill)
  const clearAiFillOnPlate = usePlannerUI((s) => s.clearAiFillOnPlate)
  const aiFilledIds = useMemo(() => new Set(aiFill.plateIds), [aiFill.plateIds])

  async function handlePick(component: Food) {
    if (!addTarget) return
    const target = addTarget
    setAddTarget(null)
    const targetDay = days[target.day]
    if (!targetDay) return
    try {
      if (target.plateId === null) {
        const created = await createPlate({
          date: targetDay.date,
          slot_id: target.slotId,
        })
        await addPlateComponent(created.id, {
          food_id: component.id,
          portions: 1,
        })
        void queryClient.invalidateQueries({
          queryKey: plateKeys.range(rangeFrom, rangeTo),
        })
      } else {
        await addCompMut.mutateAsync({
          plateId: target.plateId,
          input: { food_id: component.id, portions: 1 },
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
    const targetDay = days[target.day]
    if (!targetDay) return
    try {
      await applyTemplateMut.mutateAsync({
        templateId: template.id,
        input: { start_date: targetDay.date, slot_id: target.slotId },
      })
    } catch (err) {
      toastError(err, t)
    }
  }

  async function handleSwapPick(component: Food) {
    if (!swapTarget) return
    const target = swapTarget
    setSwapTarget(null)
    try {
      await swapMut.mutateAsync({
        plateId: target.plateId,
        pcId: target.pcId,
        input: { food_id: component.id },
      })
    } catch (err) {
      toastError(err, t)
    }
  }

  function handleDeletePlate(plateId: number, dayIdx: number) {
    if (pendingDeletesRef.current.has(plateId)) return

    const plateSnapshot = days[dayIdx]?.plates.find((p) => p.id === plateId)
    if (!plateSnapshot) return

    const timeoutId = setTimeout(async () => {
      pendingDeletesRef.current.delete(plateId)
      try {
        await deletePlateMut.mutateAsync(plateId)
      } catch (err) {
        toastError(err, t)
      }
    }, 5000)

    pendingDeletesRef.current.set(plateId, {
      timeoutId,
      snapshot: plateSnapshot,
    })

    toast(t("plate.deleted"), {
      action: {
        label: t("common.undo"),
        onClick: () => {
          const pending = pendingDeletesRef.current.get(plateId)
          if (!pending) return
          clearTimeout(pending.timeoutId)
          pendingDeletesRef.current.delete(plateId)
          void queryClient.invalidateQueries({
            queryKey: plateKeys.range(rangeFrom, rangeTo),
          })
        },
      },
      duration: 5000,
    })
  }

  function handleClearDay(dayIdx: number) {
    const targetDay = days[dayIdx]
    if (!targetDay || targetDay.plates.length === 0) return
    const dayPlates = targetDay.plates

    const timeoutId = setTimeout(async () => {
      try {
        await Promise.all(dayPlates.map((p) => deletePlate(p.id)))
      } catch (err) {
        toastError(err, t)
      } finally {
        void queryClient.invalidateQueries({
          queryKey: plateKeys.range(rangeFrom, rangeTo),
        })
      }
    }, 5000)

    toast(t("planner.day_cleared"), {
      action: {
        label: t("common.undo"),
        onClick: () => {
          clearTimeout(timeoutId)
          void queryClient.invalidateQueries({
            queryKey: plateKeys.range(rangeFrom, rangeTo),
          })
        },
      },
      duration: 5000,
    })
  }

  async function handleToggleSkip(
    dayIdx: number,
    slotId: number,
    plateId: number | null
  ) {
    const targetDay = days[dayIdx]
    if (!targetDay) return
    try {
      let id = plateId
      if (id === null) {
        const created = await createPlate({
          date: targetDay.date,
          slot_id: slotId,
        })
        id = created.id
        void queryClient.invalidateQueries({
          queryKey: plateKeys.range(rangeFrom, rangeTo),
        })
      }
      const existing = findPlateInDay(targetDay, slotId)
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
  type PendingDelete = {
    timeoutId: ReturnType<typeof setTimeout>
    snapshot: Plate
  }
  const pendingDeletesRef = useRef(new Map<number, PendingDelete>())
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
    const payload: DragPayload | null = dragStartToPayload(event, !!copyHeld)
    if (!payload) modeRef.current = "move"
  }

  async function handleDragEnd(event: DragEndEvent) {
    const active = event.active
    const over = event.over
    if (!over) return
    const activeData = active.data.current as
      | { plateId?: number; day?: number; slotId?: number; date?: string }
      | undefined
    const overData = over.data.current as
      | {
          day?: number
          slotId?: number
          date?: string
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
        await updatePlateMut.mutateAsync({
          id: activeData.plateId,
          input: {
            date: overData.date,
            slot_id: overData.slotId,
          },
        })
      } else {
        const srcDayIdx = activeData.day ?? 0
        const srcDay = days[srcDayIdx]
        const src = srcDay
          ? findPlateInDay(srcDay, activeData.slotId!)
          : undefined
        if (!src) return
        const created = await createPlate({
          date: overData.date!,
          slot_id: overData.slotId!,
          note: src.note ?? undefined,
        })
        for (const pc of src.components) {
          await addPlateComponent(created.id, {
            food_id: pc.food_id,
            portions: pc.portions,
          })
        }
        void queryClient.invalidateQueries({
          queryKey: plateKeys.range(rangeFrom, rangeTo),
        })
      }
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
              {days.map((day, idx) => {
                const date = parseISO(day.date)
                const dayIsToday = isToday(date)
                const dayKey = DAY_KEYS[day.weekday] ?? DAY_KEYS[idx % 7]
                return (
                  <DayHeader
                    key={day.date}
                    idx={idx}
                    dayKey={dayKey}
                    date={date}
                    today={dayIsToday}
                    onClearDay={() => handleClearDay(idx)}
                    hasPlates={day.plates.length > 0}
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
                  {days.map((day, dayIdx) => {
                    const date = parseISO(day.date)
                    const isPast = isBefore(date, today) && !isToday(date)
                    const plate = findPlateInDay(day, slot.id)
                    return (
                      <div
                        key={`${slot.id}-${day.date}`}
                        data-today={isToday(date) ? "true" : undefined}
                        data-past={isPast ? "true" : undefined}
                        className={isPast ? "opacity-60" : undefined}
                      >
                        <DndCellWrapper
                          day={dayIdx}
                          date={day.date}
                          slotId={slot.id}
                          plate={plate}
                        >
                          <SlotCell
                            day={dayIdx}
                            slotId={slot.id}
                            plate={plate}
                            componentsById={componentsById}
                            aiFilled={plate ? aiFilledIds.has(plate.id) : false}
                            onAdd={() => openPicker(dayIdx, slot.id)}
                            onDeletePlate={() =>
                              plate && handleDeletePlate(plate.id, dayIdx)
                            }
                            onSaveAsTemplate={
                              plate ? () => setSavePlateId(plate.id) : undefined
                            }
                            onToggleFavorite={() => {
                              const hero = plate?.components
                                .slice()
                                .sort((a, b) => a.sort_order - b.sort_order)[0]
                              const heroComp = hero
                                ? componentsById.get(hero.food_id)
                                : undefined
                              void handleToggleFavorite(
                                heroComp?.id,
                                heroComp?.favorite ?? false
                              )
                              if (plate) clearAiFillOnPlate(plate.id)
                            }}
                            onToggleSkip={() => {
                              void handleToggleSkip(
                                dayIdx,
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
                      </div>
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
