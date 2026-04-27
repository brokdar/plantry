// Pure helpers that patch a cached Week object so optimistic mutations can
// preview their effect without waiting for the server. Each helper takes the
// previous Week (or undefined when the cache is empty) and returns a new Week
// with the change applied. Unit-tested in plate-patches.test.ts.

import type { Plate, PlateComponent, Week } from "@/lib/api/plates"

function mapPlate(week: Week, plateId: number, fn: (p: Plate) => Plate): Week {
  return {
    ...week,
    plates: week.plates.map((p) => (p.id === plateId ? fn(p) : p)),
  }
}

export function patchAddPlate(
  week: Week | undefined,
  plate: Plate
): Week | undefined {
  if (!week) return week
  return { ...week, plates: [...week.plates, plate] }
}

export function patchDeletePlate(
  week: Week | undefined,
  plateId: number
): Week | undefined {
  if (!week) return week
  return { ...week, plates: week.plates.filter((p) => p.id !== plateId) }
}

export function patchUpdatePlate(
  week: Week | undefined,
  plateId: number,
  changes: Partial<Pick<Plate, "day" | "slot_id" | "note">>
): Week | undefined {
  if (!week) return week
  return mapPlate(week, plateId, (p) => ({ ...p, ...changes }))
}

export function patchAddComponent(
  week: Week | undefined,
  plateId: number,
  pc: PlateComponent
): Week | undefined {
  if (!week) return week
  return mapPlate(week, plateId, (p) => ({
    ...p,
    components: [...p.components, pc],
  }))
}

export function patchSwapComponent(
  week: Week | undefined,
  pcId: number,
  newFoodId: number,
  portionsOverride?: number
): Week | undefined {
  if (!week) return week
  return {
    ...week,
    plates: week.plates.map((p) => ({
      ...p,
      components: p.components.map((pc) =>
        pc.id === pcId
          ? {
              ...pc,
              food_id: newFoodId,
              portions: portionsOverride ?? pc.portions,
            }
          : pc
      ),
    })),
  }
}

export function patchUpdateComponentPortions(
  week: Week | undefined,
  pcId: number,
  portions: number
): Week | undefined {
  if (!week) return week
  return {
    ...week,
    plates: week.plates.map((p) => ({
      ...p,
      components: p.components.map((pc) =>
        pc.id === pcId ? { ...pc, portions } : pc
      ),
    })),
  }
}

export function patchRemoveComponent(
  week: Week | undefined,
  pcId: number
): Week | undefined {
  if (!week) return week
  return {
    ...week,
    plates: week.plates.map((p) => ({
      ...p,
      components: p.components.filter((pc) => pc.id !== pcId),
    })),
  }
}

// findPlateForComponent walks the cached week to locate the plate that owns
// the given plate_component id — used by mutation hooks that only know the
// pcId.
export function findPlateForComponent(
  week: Week,
  pcId: number
): Plate | undefined {
  return week.plates.find((p) => p.components.some((pc) => pc.id === pcId))
}

// findPlateInWeek is exported so component code can locate the plate at a
// given (day, slot) without re-implementing the predicate.
export function findPlateAt(
  week: Week,
  day: number,
  slotId: number
): Plate | undefined {
  return week.plates.find((p) => p.day === day && p.slot_id === slotId)
}
