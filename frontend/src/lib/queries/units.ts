import { useQuery } from "@tanstack/react-query"

import { getUnits, type UnitDescriptor } from "@/lib/api/units"
import { unitKeys } from "./keys"

export type { UnitDescriptor }

export function useUnits() {
  return useQuery<UnitDescriptor[]>({
    queryKey: unitKeys.list(),
    queryFn: getUnits,
    staleTime: Infinity,
  })
}
