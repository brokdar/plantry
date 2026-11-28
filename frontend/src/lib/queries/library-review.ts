import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import {
  dismissLibraryReview,
  getLibraryReview,
  makeReviewVariants,
} from "@/lib/api/library-review"
import { toastError } from "@/lib/toast"

import { foodKeys, chefKeys } from "./keys"

/** This month's library review card (LIB-1..3); `enabled` is the switch. */
export function useLibraryReview(enabled: boolean) {
  return useQuery({
    queryKey: chefKeys.libraryReview(),
    queryFn: async () => (await getLibraryReview()).card,
    enabled,
  })
}

/** Dismiss a finding (or the card) and make variants of a duplicates
 *  finding; both refetch the card. */
export function useLibraryReviewActions() {
  const { t } = useTranslation()
  const qc = useQueryClient()
  const options = {
    onSuccess: () =>
      Promise.all([
        qc.invalidateQueries({ queryKey: chefKeys.libraryReview() }),
        qc.invalidateQueries({ queryKey: foodKeys.lists() }),
      ]),
    onError: (err: unknown) => toastError(err, t),
  }
  return {
    dismiss: useMutation({
      mutationFn: (key?: string) => dismissLibraryReview(key),
      ...options,
    }),
    makeVariants: useMutation({
      mutationFn: (key: string) => makeReviewVariants(key),
      ...options,
    }),
  }
}
