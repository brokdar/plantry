import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import {
  addBasic,
  getKitchen,
  removeBasic,
  updateKitchen,
  type Kitchen,
  type KitchenSettings,
} from "@/lib/api/kitchen"
import { toastError } from "@/lib/toast"

import { kitchenKeys } from "./keys"

export function useKitchen() {
  return useQuery({ queryKey: kitchenKeys.detail, queryFn: getKitchen })
}

const PATCH_KITCHEN_KEY = ["kitchen", "patch"] as const

/**
 * Saves some kitchen settings. `PUT /api/kitchen` is a full replace, so a
 * patch is merged into the cached kitchen. Unlike `usePatchProfile` the cache
 * changes in the call itself, not in `onMutate` (which waits its turn in the
 * serial queue): its lists are edited from the cached value, so a second
 * click right after the first must already see it. A failure toasts and
 * refetches.
 */
export function usePatchKitchen() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  const m = useMutation({
    mutationKey: PATCH_KITCHEN_KEY,
    scope: { id: "kitchen" },
    // The cache already holds every queued patch, so each request sends the
    // latest kitchen.
    mutationFn: (patch: Partial<KitchenSettings>) => {
      const c = qc.getQueryData<Kitchen>(kitchenKeys.detail)
      return updateKitchen({
        minutes_weekday: c?.minutes_weekday ?? 30,
        minutes_weekend: c?.minutes_weekend ?? 60,
        skill: c?.skill ?? null,
        equipment: c?.equipment ?? [],
        heat: c?.heat ?? null,
        leanings: c?.leanings ?? [],
        ...patch,
      })
    },
    onSuccess: (data) => {
      if (qc.isMutating({ mutationKey: PATCH_KITCHEN_KEY }) <= 1) {
        qc.setQueryData(kitchenKeys.detail, data)
      }
    },
    onError: (err) => {
      toastError(err, t)
      void qc.invalidateQueries({ queryKey: kitchenKeys.detail })
    },
  })
  const apply = (patch: Partial<KitchenSettings>) => {
    void qc.cancelQueries({ queryKey: kitchenKeys.detail })
    qc.setQueryData<Kitchen>(kitchenKeys.detail, (prev) =>
      prev ? { ...prev, ...patch } : prev
    )
  }
  return {
    mutate: (
      patch: Partial<KitchenSettings>,
      options?: Parameters<typeof m.mutate>[1]
    ) => {
      apply(patch)
      m.mutate(patch, options)
    },
    mutateAsync: (patch: Partial<KitchenSettings>) => {
      apply(patch)
      return m.mutateAsync(patch)
    },
  }
}

/** Marks a food as always at home, or takes it off the list. */
export function useSetBasic() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  return useMutation({
    mutationFn: ({ id, atHome }: { id: number; atHome: boolean }) =>
      atHome ? addBasic(id) : removeBasic(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: kitchenKeys.detail }),
    onError: (err) => toastError(err, t),
  })
}
