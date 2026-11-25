import { useQuery } from "@tanstack/react-query"

import { getChefSpend } from "@/lib/api/chef"

import { chefKeys } from "./keys"

/** This month's capped spend against the cap. */
export function useChefSpend() {
  return useQuery({
    queryKey: chefKeys.spend(),
    queryFn: getChefSpend,
  })
}
