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
import { shoppingKeys } from "@/lib/queries/shopping"
import type { Food } from "@/lib/api/foods"
import type { Plate } from "@/lib/api/plates"
import type { TimeSlot } from "@/lib/api/slots"
import type { NutritionDay } from "@/lib/api/nutrition"
import { useFoods, useSetFoodFavorite } from "@/lib/queries/foods"
import { useClearFeedback, useRecordFeedback } from "@/lib/queries/feedback"
import {
  useDeletePlate,
  useSetPlateSkipped,
  useSwapPlateComponent,
  useUpdatePlate,
} from "@/lib/queries/plates"
import { slotLabel } from "@/lib/slot-label"
import { usePlannerUI } from "@/lib/stores/planner-ui"
import { toast, toastError } from "@/lib/toast"

import { AddComponentSheet } from "./AddComponentSheet"
import { ComponentTraySheet, type TraySlotContext } from "./ComponentTraySheet"
import { DayHeader } from "./DayHeader"
import { SlotCell } from "./SlotCell"
import { SlotSheet, type SlotSheetTarget } from "./SlotSheet"

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
  nutritionDays?: NutritionDay[]
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

function buildTrayContext(
  target: AddTarget,
  days: PlannerDay[],
  slotsById: Map<number, TimeSlot>
): TraySlotContext | null {
  const day = days[target.day]
  const slot = slotsById.get(target.slotId)
  if (!day || !slot) return null
  return {
    slotId: slot.id,
    slotNameKey: slot.name_key,
    date: day.date,
    weekday: day.weekday,
  }
}

export function PlannerGrid({
  days,
  slots,
  rangeFrom,
  rangeTo,
  nutritionDays,
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

  const slotsById = useMemo(() => {
    const map = new Map<number, TimeSlot>()
    for (const s of slots) map.set(s.id, s)
    return map
  }, [slots])

  // Frontend-derived "Recent" tab content for the tray sheet. Walks the
  // plates currently in cache (the visible window), de-duplicates by food id
  // and orders most-recent-first by plate date. Capped at 20 to keep the
  // tab focused. Cheap because it only iterates visible plates — the heavy
  // foods catalog already lives in `componentsById`.
  const recentFoods = useMemo(() => {
    const seen = new Set<number>()
    const out: Food[] = []
    const sortedDays = [...days].sort((a, b) =>
      a.date < b.date ? 1 : a.date > b.date ? -1 : 0
    )
    for (const day of sortedDays) {
      for (const plate of day.plates) {
        for (const pc of plate.components) {
          if (seen.has(pc.food_id)) continue
          const food = componentsById.get(pc.food_id)
          if (!food) continue
          seen.add(pc.food_id)
          out.push(food)
          if (out.length >= 20) return out
        }
      }
    }
    return out
  }, [days, componentsById])

  const today = startOfDay(new Date())

  const [addTarget, setAddTarget] = useState<AddTarget | null>(null)
  const [swapTarget, setSwapTarget] = useState<SwapTarget | null>(null)
  const [savePlateId, setSavePlateId] = useState<number | null>(null)
  const [sheetTarget, setSheetTarget] = useState<SlotSheetTarget | null>(null)

  function openSheetForPlate(dayIdx: number, slot: TimeSlot, plate: Plate) {
    const day = days[dayIdx]
    if (!day) return
    setSheetTarget({
      plateId: plate.id,
      date: day.date,
      weekday: day.weekday,
      slotId: slot.id,
      slotNameKey: slot.name_key,
    })
  }

  const updatePlateMut = useUpdatePlate(rangeFrom, rangeTo)
  const swapMut = useSwapPlateComponent()
  const deletePlateMut = useDeletePlate()
  const setSkippedMut = useSetPlateSkipped()
  const setFavoriteMut = useSetFoodFavorite()
  const recordFeedbackMut = useRecordFeedback()
  const clearFeedbackMut = useClearFeedback()

  const aiFill = usePlannerUI((s) => s.aiFill)
  const clearAiFillOnPlate = usePlannerUI((s) => s.clearAiFillOnPlate)
  const aiFilledIds = useMemo(() => new Set(aiFill.plateIds), [aiFill.plateIds])

  async function handleTrayCommit(
    items: { food_id: number; portions: number }[]
  ): Promise<{ failedFoodIds: number[] }> {
    if (!addTarget || items.length === 0) return { failedFoodIds: [] }
    const target = addTarget
    const targetDay = days[target.day]
    if (!targetDay) return { failedFoodIds: [] }

    let plateId = target.plateId
    if (plateId === null) {
      // If creating the plate fails, nothing landed — every staged item
      // counts as failed so the tray keeps them all for retry.
      try {
        const created = await createPlate({
          date: targetDay.date,
          slot_id: target.slotId,
        })
        plateId = created.id
      } catch (err) {
        toastError(err, t)
        return { failedFoodIds: items.map((i) => i.food_id) }
      }
    }

    // Add components in parallel and survive partial failure: report what
    // didn't land so the tray sheet can keep those staged for retry.
    const results = await Promise.allSettled(
      items.map((it) =>
        addPlateComponent(plateId!, {
          food_id: it.food_id,
          portions: it.portions,
        })
      )
    )
    const failedFoodIds: number[] = []
    let firstError: unknown
    results.forEach((r, idx) => {
      if (r.status === "rejected") {
        failedFoodIds.push(items[idx]!.food_id)
        firstError ??= r.reason
      }
    })
    void queryClient.invalidateQueries({
      queryKey: plateKeys.range(rangeFrom, rangeTo),
    })
    void queryClient.invalidateQueries({ queryKey: shoppingKeys.all })

    const ok = items.length - failedFoodIds.length
    if (failedFoodIds.length === 0) {
      toast(
        ok === 1
          ? t("tray.committed_one")
          : t("tray.committed_other", { count: ok })
      )
    } else if (ok > 0) {
      toast(
        failedFoodIds.length === 1
          ? t("tray.partial_failure_one")
          : t("tray.partial_failure_other", { count: failedFoodIds.length })
      )
    } else {
      toastError(firstError, t)
    }
    return { failedFoodIds }
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

    // Optimistic: remove from cache immediately so the UI updates at once.
    queryClient.setQueryData<{ plates: Plate[] }>(
      plateKeys.range(rangeFrom, rangeTo),
      (old) => ({ plates: (old?.plates ?? []).filter((p) => p.id !== plateId) })
    )

    const timeoutId = setTimeout(async () => {
      pendingDeletesRef.current.delete(plateId)
      try {
        await deletePlateMut.mutateAsync(plateId)
      } catch (err) {
        toastError(err, t)
        queryClient.setQueryData<{ plates: Plate[] }>(
          plateKeys.range(rangeFrom, rangeTo),
          (old) => ({ plates: [...(old?.plates ?? []), plateSnapshot] })
        )
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
          queryClient.setQueryData<{ plates: Plate[] }>(
            plateKeys.range(rangeFrom, rangeTo),
            (old) => ({ plates: [...(old?.plates ?? []), pending.snapshot] })
          )
        },
      },
      duration: 5000,
    })
  }

  function handleClearDay(dayIdx: number) {
    const targetDay = days[dayIdx]
    if (!targetDay || targetDay.plates.length === 0) return
    const dayPlates = targetDay.plates

    // Optimistic: remove day's plates from cache immediately.
    const dayPlateIds = new Set(dayPlates.map((p) => p.id))
    queryClient.setQueryData<{ plates: Plate[] }>(
      plateKeys.range(rangeFrom, rangeTo),
      (old) => ({
        plates: (old?.plates ?? []).filter((p) => !dayPlateIds.has(p.id)),
      })
    )

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
          queryClient.setQueryData<{ plates: Plate[] }>(
            plateKeys.range(rangeFrom, rangeTo),
            (old) => ({ plates: [...(old?.plates ?? []), ...dayPlates] })
          )
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
  // PointerSensor's 6 px activation distance disambiguates click-to-open from
  // drag-to-reschedule on the SlotCell stretched-link button (the same button
  // is both click target and drag activator via setActivatorNodeRef).
  // KeyboardSensor's default start keys are Space/Enter — the same keys that
  // fire <button> click. dnd-kit calls preventDefault on the keydown when the
  // focused element is the activator, so keyboard users tabbing to the cell
  // and pressing Enter start a drag instead of opening the sheet. They keep
  // full keyboard access via the cell's dropdown menu (Add / Skip / Save /
  // Delete), so this is a missed opportunity rather than a regression. Phase 7
  // of the redesign adds an explicit grip-dots drag handle, which separates
  // the activator from the click target and resolves the conflict naturally.
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
                const dayMacros = nutritionDays?.find(
                  (n) => n.date === day.date
                )?.macros
                return (
                  <DayHeader
                    key={day.date}
                    idx={idx}
                    dayKey={dayKey}
                    date={date}
                    today={dayIsToday}
                    macros={dayMacros}
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
                          {(dragHandle) => (
                            <SlotCell
                              day={dayIdx}
                              slotId={slot.id}
                              plate={plate}
                              componentsById={componentsById}
                              aiFilled={
                                plate ? aiFilledIds.has(plate.id) : false
                              }
                              dragHandle={dragHandle}
                              onAdd={() => openPicker(dayIdx, slot.id)}
                              onOpenSheet={
                                plate
                                  ? () => openSheetForPlate(dayIdx, slot, plate)
                                  : undefined
                              }
                              onDeletePlate={() =>
                                plate && handleDeletePlate(plate.id, dayIdx)
                              }
                              onSaveAsTemplate={
                                plate
                                  ? () => setSavePlateId(plate.id)
                                  : undefined
                              }
                              onToggleFavorite={() => {
                                const hero = plate?.components
                                  .slice()
                                  .sort(
                                    (a, b) => a.sort_order - b.sort_order
                                  )[0]
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
                          )}
                        </DndCellWrapper>
                      </div>
                    )
                  })}
                </div>
              ))}
            </div>
          </div>
        </div>

        <ComponentTraySheet
          open={addTarget !== null}
          context={
            addTarget ? buildTrayContext(addTarget, days, slotsById) : null
          }
          recentFoods={recentFoods}
          onOpenChange={(o) => !o && setAddTarget(null)}
          onCommit={(items) => handleTrayCommit(items)}
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
        <SlotSheet
          target={sheetTarget}
          days={days}
          componentsById={componentsById}
          rangeFrom={rangeFrom}
          rangeTo={rangeTo}
          aiFilled={sheetTarget ? aiFilledIds.has(sheetTarget.plateId) : false}
          onOpenChange={(open) => {
            if (!open) setSheetTarget(null)
          }}
          onAddComponent={(target) =>
            setAddTarget({
              day: days.findIndex((d) => d.date === target.date),
              slotId: target.slotId,
              plateId: target.plateId,
            })
          }
          onSwapComponent={(target, pcId, defaultRole) =>
            setSwapTarget({
              plateId: target.plateId,
              pcId,
              defaultRole,
            })
          }
          onSaveAsTemplate={(plateId) => setSavePlateId(plateId)}
          onToggleSkip={(target, currentSkipped) => {
            const dayIdx = days.findIndex((d) => d.date === target.date)
            if (dayIdx < 0) return
            void handleToggleSkip(dayIdx, target.slotId, target.plateId)
            if (currentSkipped) {
              // Removing skip — keep sheet open for further edits.
              return
            }
            // Marking skip — close the sheet so the user sees the cell update.
            setSheetTarget(null)
          }}
          onDeletePlate={(plateId) => {
            const dayIdx = days.findIndex((d) =>
              d.plates.some((p) => p.id === plateId)
            )
            if (dayIdx < 0) return
            handleDeletePlate(plateId, dayIdx)
            setSheetTarget(null)
          }}
        />
      </div>
    </DndContext>
  )
}
