// Ephemeral planner UI state. NEVER mirror server data here — all server
// state lives in TanStack Query. This store only tracks transient UI:
// which plate is being edited, which empty cell the "add" sheet is targeting,
// and the current drag state.

import { create } from "zustand"

export interface AddingTarget {
  day: number
  slotId: number
}

interface PlannerUIState {
  editingPlateId: number | null
  addingTo: AddingTarget | null
  swapTarget: { plateId: number; pcId: number; role?: string } | null

  openEditor: (plateId: number) => void
  closeEditor: () => void
  beginAdd: (target: AddingTarget) => void
  cancelAdd: () => void
  beginSwap: (target: { plateId: number; pcId: number; role?: string }) => void
  cancelSwap: () => void
}

export const usePlannerUI = create<PlannerUIState>((set) => ({
  editingPlateId: null,
  addingTo: null,
  swapTarget: null,

  openEditor: (plateId) => set({ editingPlateId: plateId }),
  closeEditor: () => set({ editingPlateId: null }),
  beginAdd: (target) => set({ addingTo: target }),
  cancelAdd: () => set({ addingTo: null }),
  beginSwap: (target) => set({ swapTarget: target }),
  cancelSwap: () => set({ swapTarget: null }),
}))
