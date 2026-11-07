import { keepPreviousData, useQuery } from "@tanstack/react-query"

import { getToday } from "@/lib/api/today"

import { todayKeys } from "./keys"

export function useToday(date: string, today: string) {
  return useQuery({
    queryKey: todayKeys.date(date, today),
    queryFn: () => getToday(date, today),
    enabled: !!date,
    // Stepping to another day keeps the header (and the focused arrow) on
    // screen while the new day loads.
    placeholderData: keepPreviousData,
  })
}
