import { useMutation, useQuery } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import {
  fillLibrary,
  getChefSpend,
  getLabellingEstimate,
  startLibraryLabelling,
} from "@/lib/api/chef"
import { toast, toastError } from "@/lib/toast"

import { chefKeys } from "./keys"

/** This month's capped spend against the cap. */
export function useChefSpend() {
  return useQuery({
    queryKey: chefKeys.spend(),
    queryFn: getChefSpend,
  })
}

/** The library labelling estimate, read while `enabled` (the dialog open). */
export function useLabellingEstimate(enabled: boolean) {
  return useQuery({
    queryKey: chefKeys.labelling(),
    queryFn: getLabellingEstimate,
    enabled,
    staleTime: 0,
  })
}

/** Queues the one-time library labelling. */
export function useStartLibraryLabelling() {
  const { t } = useTranslation()
  return useMutation({
    mutationFn: startLibraryLabelling,
    onError: (err) => toastError(err, t),
  })
}

/** "Fill my library": starts ten finds that arrive in the library on trial. */
export function useFillLibrary() {
  const { t } = useTranslation()
  return useMutation({
    mutationFn: fillLibrary,
    onSuccess: () => toast(t("recipes.fill.started")),
    onError: (err) => toastError(err, t),
  })
}
