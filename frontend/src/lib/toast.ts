import type { TFunction } from "i18next"
import { toast } from "sonner"

import { ApiError } from "@/lib/api/client"

export function toastError(
  err: unknown,
  t: TFunction,
  fallbackMessage?: string
) {
  // ApiError carries a translated key (e.g. "error.network"). When the caller
  // passes `fallbackMessage`, use it for any non-API error so the user sees
  // an action-specific message instead of the generic "Something went wrong".
  if (err instanceof ApiError) {
    toast.error(t(err.messageKey))
    return
  }
  toast.error(fallbackMessage ?? t("error.server"))
}

export function toastSuccess(messageKey: string, t: TFunction) {
  toast.success(t(messageKey))
}

export { toast }
