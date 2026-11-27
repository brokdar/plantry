import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useEffect } from "react"
import { useTranslation } from "react-i18next"

import {
  getAssistantDid,
  planOpened,
  undoPlacedFind,
} from "@/lib/api/assistant-did"
import { todayISO } from "@/lib/dates"
import { toast, toastError } from "@/lib/toast"

import { foodKeys, planKeys } from "./keys"
import { invalidatePlanViews } from "./plan"

/** Under planKeys.all: every plan change refetches the list. */
export const assistantDidKeys = {
  window: (start: string) => [...planKeys.all, "assistant-did", start] as const,
}

/** The window's "What the assistant did" list (empty with the Chef off). */
export function useAssistantDid(start: string, enabled: boolean) {
  return useQuery({
    queryKey: assistantDidKeys.window(start),
    queryFn: () => getAssistantDid(start),
    enabled,
  })
}

/** Tells the server the plan page shows `start` (a planning session, P11).
 *  Once per window the page shows, only with the Chef on. */
export function usePlanOpened(start: string, chefOn: boolean) {
  useEffect(() => {
    if (!chefOn) return
    void planOpened(start, todayISO()).catch(() => undefined)
  }, [start, chefOn])
}

/** Undo a placed find: its plates leave the window and it is ready again. */
export function useUndoPlacedFind() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  return useMutation({
    mutationFn: (id: number) => undoPlacedFind(id),
    onSuccess: () => {
      toast.success(t("plan.assistant_did.undone"))
      void qc.invalidateQueries({ queryKey: foodKeys.lists() })
      return invalidatePlanViews(qc)
    },
    onError: (err) => {
      void invalidatePlanViews(qc)
      toastError(err, t)
    },
  })
}
