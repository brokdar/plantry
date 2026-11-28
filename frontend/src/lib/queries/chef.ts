import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import {
  addChefSite,
  fillLibrary,
  getChefSpend,
  getLabellingEstimate,
  listChefSites,
  removeChefSite,
  retryChefSite,
  setChefSiteEnabled,
  startLibraryLabelling,
  type ChefSite,
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

/** The recipe sites finds search (FIND-7). */
export function useChefSites() {
  return useQuery({
    queryKey: chefKeys.sites(),
    queryFn: async () => (await listChefSites()).items,
  })
}

/** Site mutations: each answers with the whole list, which replaces the
 *  cached one. */
export function useChefSiteActions() {
  const { t } = useTranslation()
  const qc = useQueryClient()
  const options = {
    onSuccess: (r: { items: ChefSite[] }) =>
      qc.setQueryData(chefKeys.sites(), r.items),
    onError: (err: unknown) => toastError(err, t),
  }
  return {
    add: useMutation({
      mutationFn: (input: { domain: string; lang: "en" | "de" }) =>
        addChefSite(input),
      ...options,
    }),
    toggle: useMutation({
      mutationFn: ({ domain, enabled }: { domain: string; enabled: boolean }) =>
        setChefSiteEnabled(domain, enabled),
      ...options,
    }),
    remove: useMutation({
      mutationFn: (domain: string) => removeChefSite(domain),
      ...options,
    }),
    retry: useMutation({
      mutationFn: (domain: string) => retryChefSite(domain),
      ...options,
    }),
  }
}
