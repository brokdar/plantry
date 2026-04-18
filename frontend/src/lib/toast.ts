import type { TFunction } from "i18next"
import { toast } from "sonner"

import { ApiError } from "@/lib/api/client"

export function toastError(err: unknown, t: TFunction) {
  const messageKey = err instanceof ApiError ? err.messageKey : "error.server"
  toast.error(t(messageKey))
}

export function toastSuccess(messageKey: string, t: TFunction) {
  toast.success(t(messageKey))
}

export { toast }
