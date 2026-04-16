import { useQuery, useMutation } from "@tanstack/react-query"

import {
  getCurrentWeek,
  getWeek,
  getWeekByDate,
  getShoppingList,
  getWeekNutrition,
  copyWeek,
  createPlate,
  type CopyWeekInput,
  type CreatePlateInput,
} from "@/lib/api/weeks"
import { queryClient } from "@/lib/query-client"

import { weekKeys } from "./keys"

export function useCurrentWeek() {
  return useQuery({
    queryKey: weekKeys.current(),
    queryFn: getCurrentWeek,
  })
}

export function useWeek(id: number) {
  return useQuery({
    queryKey: weekKeys.byId(id),
    queryFn: () => getWeek(id),
    enabled: id > 0,
  })
}

export function useWeekByDate(year: number, week: number) {
  return useQuery({
    queryKey: weekKeys.byDate(year, week),
    queryFn: () => getWeekByDate(year, week),
    enabled: year > 0 && week > 0,
  })
}

export function useCopyWeek() {
  return useMutation({
    mutationFn: ({ id, input }: { id: number; input: CopyWeekInput }) =>
      copyWeek(id, input),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: weekKeys.all })
    },
  })
}

export function useShoppingList(weekId: number) {
  return useQuery({
    queryKey: weekKeys.shoppingList(weekId),
    queryFn: () => getShoppingList(weekId),
    enabled: weekId > 0,
  })
}

export function useWeekNutrition(weekId: number) {
  return useQuery({
    queryKey: weekKeys.nutrition(weekId),
    queryFn: () => getWeekNutrition(weekId),
    enabled: weekId > 0,
  })
}

export function useCreatePlate(weekId: number) {
  return useMutation({
    mutationFn: (input: CreatePlateInput) => createPlate(weekId, input),
    onSuccess: () => {
      // weekId scope unused here — invalidating all week queries covers
      // byDate, byId, and current cache slots for the affected week.
      void queryClient.invalidateQueries({ queryKey: weekKeys.all })
      void { weekId } // silence unused-arg lint while keeping API stable
    },
  })
}
