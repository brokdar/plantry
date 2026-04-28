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
//
// aiFill is keyed by date range (from/to) instead of weekId so it works for
// any 7-day window, not just ISO-week-aligned windows.
export interface AiFillSession {
  range: { from: string; to: string } | null
  startedAt: number | null
  plateIds: number[]
  dismissed: boolean
}

interface PlannerUIState {
  editingPlateId: number | null
  addingTo: AddingTarget | null
  swapTarget: { plateId: number; pcId: number; role?: string } | null
  aiFill: AiFillSession

  openEditor: (plateId: number) => void
  closeEditor: () => void
  beginAdd: (target: AddingTarget) => void
  cancelAdd: () => void
  beginSwap: (target: { plateId: number; pcId: number; role?: string }) => void
  cancelSwap: () => void

  startAiFill: (range: { from: string; to: string }) => void
  recordAiFilledPlate: (plateId: number) => void
  clearAiFillOnPlate: (plateId: number) => void
  dismissAiFillBanner: () => void
  endAiFillSession: () => void
}

export const usePlannerUI = create<PlannerUIState>((set) => ({
  editingPlateId: null,
  addingTo: null,
  swapTarget: null,
  aiFill: { range: null, startedAt: null, plateIds: [], dismissed: false },

  openEditor: (plateId) => set({ editingPlateId: plateId }),
  closeEditor: () => set({ editingPlateId: null }),
  beginAdd: (target) => set({ addingTo: target }),
  cancelAdd: () => set({ addingTo: null }),
  beginSwap: (target) => set({ swapTarget: target }),
  cancelSwap: () => set({ swapTarget: null }),

  startAiFill: (range) =>
    set({
      aiFill: { range, startedAt: Date.now(), plateIds: [], dismissed: false },
    }),
  recordAiFilledPlate: (plateId) =>
    set((state) => ({
      aiFill: {
        ...state.aiFill,
        plateIds: state.aiFill.plateIds.includes(plateId)
          ? state.aiFill.plateIds
          : [...state.aiFill.plateIds, plateId],
      },
    })),
  clearAiFillOnPlate: (plateId) =>
    set((state) => ({
      aiFill: {
        ...state.aiFill,
        plateIds: state.aiFill.plateIds.filter((id) => id !== plateId),
      },
    })),
  dismissAiFillBanner: () =>
    set((state) => ({ aiFill: { ...state.aiFill, dismissed: true } })),
  endAiFillSession: () =>
    set({
      aiFill: { range: null, startedAt: null, plateIds: [], dismissed: false },
    }),
}))
