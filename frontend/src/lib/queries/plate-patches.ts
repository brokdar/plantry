// Pure helpers that patch a cached Week object so optimistic mutations can
// preview their effect without waiting for the server. Each helper takes the
// previous Week (or undefined when the cache is empty) and returns a new Week
// with the change applied. Unit-tested in plate-patches.test.ts.

import type {
  Plate,
  PlateComponent,
  PlateComponentQuantity,
  Week,
} from "@/lib/api/plates"

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
  changes: Partial<Pick<Plate, "slot_id" | "note">>
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

/**
 * patchUpdateComponentQuantity applies a kind-aware quantity patch:
 *  - composed → sets `portions`, clears `amount/unit/grams`.
 *  - leaf     → sets `amount/unit`, clears `portions`. `grams` will be
 *               re-resolved by the server; the optimistic patch leaves the
 *               cached `grams`/`grams_source` alone so the row can keep
 *               showing the previous resolution until the refetch lands.
 */
export function patchUpdateComponentQuantity(
  week: Week | undefined,
  pcId: number,
  quantity: PlateComponentQuantity
): Week | undefined {
  if (!week) return week
  return {
    ...week,
    plates: week.plates.map((p) => ({
      ...p,
      components: p.components.map((pc) => {
        if (pc.id !== pcId) return pc
        if ("portions" in quantity) {
          return {
            ...pc,
            portions: quantity.portions,
            amount: undefined,
            unit: undefined,
            grams: undefined,
            grams_source: undefined,
          }
        }
        return {
          ...pc,
          portions: undefined,
          amount: quantity.amount,
          unit: quantity.unit,
        }
      }),
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

// findPlateAt locates the plate at a given (date, slot) in the cached week.
export function findPlateAt(
  week: Week,
  date: string,
  slotId: number
): Plate | undefined {
  return week.plates.find((p) => p.date === date && p.slot_id === slotId)
}
