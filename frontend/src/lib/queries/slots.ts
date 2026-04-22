import { useQuery, useMutation } from "@tanstack/react-query"

import {
  listTimeSlots,
  createTimeSlot,
  updateTimeSlot,
  deleteTimeSlot,
  type TimeSlotInput,
} from "@/lib/api/slots"
import { queryClient } from "@/lib/query-client"

import { slotKeys } from "./keys"

export function useTimeSlots(activeOnly = false) {
  return useQuery({
    queryKey: slotKeys.list(activeOnly),
    queryFn: () => listTimeSlots(activeOnly),
  })
}

export function useCreateTimeSlot() {
  return useMutation({
    mutationFn: (input: TimeSlotInput) => createTimeSlot(input),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: slotKeys.lists() })
    },
  })
}

export function useUpdateTimeSlot() {
  return useMutation({
    mutationFn: ({ id, data }: { id: number; data: TimeSlotInput }) =>
      updateTimeSlot(id, data),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: slotKeys.lists() })
    },
  })
}

export function useDeleteTimeSlot() {
  return useMutation({
    mutationFn: (id: number) => deleteTimeSlot(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: slotKeys.lists() })
    },
  })
}
