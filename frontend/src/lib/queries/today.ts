import { useQuery } from "@tanstack/react-query"

import { getToday } from "@/lib/api/today"

import { todayKeys } from "./keys"

export function useToday(date: string) {
  return useQuery({
    queryKey: todayKeys.date(date),
    queryFn: () => getToday(date),
    enabled: !!date,
  })
}
