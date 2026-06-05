import { useQuery, useMutation } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import {
  listTimeSlots,
  createTimeSlot,
  updateTimeSlot,
  deleteTimeSlot,
  type TimeSlotInput,
} from "@/lib/api/slots"
import { queryClient } from "@/lib/query-client"
import { toastError } from "@/lib/toast"

import { slotKeys } from "./keys"

export function useTimeSlots(activeOnly = false) {
  return useQuery({
    queryKey: slotKeys.list(activeOnly),
    queryFn: () => listTimeSlots(activeOnly),
  })
}

export function useCreateTimeSlot() {
  const { t } = useTranslation()
  return useMutation({
    mutationFn: (input: TimeSlotInput) => createTimeSlot(input),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: slotKeys.lists() })
    },
    onError: (err) => toastError(err, t),
  })
}

export function useUpdateTimeSlot() {
  const { t } = useTranslation()
  return useMutation({
    mutationFn: ({ id, data }: { id: number; data: TimeSlotInput }) =>
      updateTimeSlot(id, data),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: slotKeys.lists() })
    },
    onError: (err) => toastError(err, t),
  })
}

export function useDeleteTimeSlot() {
  const { t } = useTranslation()
  return useMutation({
    mutationFn: (id: number) => deleteTimeSlot(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: slotKeys.lists() })
    },
    onError: (err) => toastError(err, t),
  })
}
