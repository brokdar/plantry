import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import { applyTip, dismissTip, getChefInsights, getTips } from "@/lib/api/tips"
import { toastError } from "@/lib/toast"

import { planKeys } from "./keys"
import { invalidatePlanViews, useRevertUndo } from "./plan"

/** Under planKeys.all: every plan change refetches the tips. */
export const tipKeys = {
  window: (date: string) => [...planKeys.all, "tips", date] as const,
  insights: ["chef", "insights"] as const,
}

/** The live tips and plate fit of the window that holds `date`. */
export function useTips(date: string | undefined) {
  return useQuery({
    queryKey: tipKeys.window(date ?? ""),
    queryFn: () => getTips(date!),
    enabled: !!date,
  })
}

/** Apply a tip: a normal plate change with the shared revert Undo. */
export function useApplyTip() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  const offerRevert = useRevertUndo()
  return useMutation({
    mutationFn: (id: number) => applyTip(id),
    onSuccess: (res) => {
      offerRevert(res.undo, t("tips.applied"))
      return invalidatePlanViews(qc)
    },
    onError: (err) => {
      void invalidatePlanViews(qc)
      toastError(err, t)
    },
  })
}

/** Dismiss a tip (it cools down for the recipe in this window). */
export function useDismissTip() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  return useMutation({
    mutationFn: (id: number) => dismissTip(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: planKeys.all }),
    onError: (err) => toastError(err, t),
  })
}

/** The Chef block of Insights. */
export function useChefInsights() {
  return useQuery({ queryKey: tipKeys.insights, queryFn: getChefInsights })
}
