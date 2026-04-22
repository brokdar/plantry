// Ephemeral planner UI state. NEVER mirror server data here — all server
// state lives in TanStack Query. This store only tracks transient UI:
// which plate is being edited, which empty cell the "add" sheet is targeting,
// and the current drag state.

import { create } from "zustand"

export interface AddingTarget {
  day: number
  slotId: number
}

// AI-fill session state lives here too. Tracking this client-side (not on the
// plate row) matches the product decision that the gold-leaf badge is a
// transient session marker — a hard refresh clears it, a manual edit clears
// the marker on that specific plate only.
export interface AiFillSession {
  weekId: number
  snapshotWeekId: number
  startedAt: number
  aiFilledPlateIds: number[]
  dismissed: boolean
}

interface PlannerUIState {
  editingPlateId: number | null
  addingTo: AddingTarget | null
  swapTarget: { plateId: number; pcId: number; role?: string } | null
  aiFill: AiFillSession | null

  openEditor: (plateId: number) => void
  closeEditor: () => void
  beginAdd: (target: AddingTarget) => void
  cancelAdd: () => void
  beginSwap: (target: { plateId: number; pcId: number; role?: string }) => void
  cancelSwap: () => void

  startAiFill: (
    s: Omit<AiFillSession, "aiFilledPlateIds" | "dismissed">
  ) => void
  recordAiFilledPlate: (plateId: number) => void
  clearAiFillOnPlate: (plateId: number) => void
  dismissAiFillBanner: () => void
  endAiFillSession: () => void
}

export const usePlannerUI = create<PlannerUIState>((set) => ({
  editingPlateId: null,
  addingTo: null,
  swapTarget: null,
  aiFill: null,

  openEditor: (plateId) => set({ editingPlateId: plateId }),
  closeEditor: () => set({ editingPlateId: null }),
  beginAdd: (target) => set({ addingTo: target }),
  cancelAdd: () => set({ addingTo: null }),
  beginSwap: (target) => set({ swapTarget: target }),
  cancelSwap: () => set({ swapTarget: null }),

  startAiFill: (s) =>
    set({ aiFill: { ...s, aiFilledPlateIds: [], dismissed: false } }),
  recordAiFilledPlate: (plateId) =>
    set((state) =>
      state.aiFill
        ? {
            aiFill: {
              ...state.aiFill,
              aiFilledPlateIds: state.aiFill.aiFilledPlateIds.includes(plateId)
                ? state.aiFill.aiFilledPlateIds
                : [...state.aiFill.aiFilledPlateIds, plateId],
            },
          }
        : state
    ),
  clearAiFillOnPlate: (plateId) =>
    set((state) =>
      state.aiFill
        ? {
            aiFill: {
              ...state.aiFill,
              aiFilledPlateIds: state.aiFill.aiFilledPlateIds.filter(
                (id) => id !== plateId
              ),
            },
          }
        : state
    ),
  dismissAiFillBanner: () =>
    set((state) =>
      state.aiFill ? { aiFill: { ...state.aiFill, dismissed: true } } : state
    ),
  endAiFillSession: () => set({ aiFill: null }),
}))
