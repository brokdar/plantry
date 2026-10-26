import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import { completeSetup, getSetupStatus } from "@/lib/api/setup"
import { toastError } from "@/lib/toast"

import { setupKeys } from "./keys"

export { setupKeys }

/** `GET /api/setup`: completion flag and the recipe count vs target. */
export function useSetupStatus() {
  return useQuery({
    queryKey: setupKeys.status(),
    queryFn: getSetupStatus,
  })
}

/** `POST /api/setup/complete`: marks the wizard done. */
export function useCompleteSetup() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  return useMutation({
    mutationFn: completeSetup,
    onSuccess: (data) => {
      qc.setQueryData(setupKeys.status(), data)
      void qc.invalidateQueries({ queryKey: setupKeys.status() })
    },
    onError: (err) => toastError(err, t),
  })
}
