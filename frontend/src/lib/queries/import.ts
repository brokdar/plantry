import { useMutation, useQuery } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
import {
  checkImportDraft,
  extractRecipe,
  lookupImportLine,
  resolveImport,
  type Draft,
  type ExtractRequest,
  type ResolveRequest,
} from "@/lib/api/import"
import { toastError } from "@/lib/toast"
import { importKeys } from "./keys"

export function useExtractRecipe() {
  return useMutation({
    mutationFn: (req: ExtractRequest) => extractRecipe(req),
  })
}

/** The import check over a Discover suggestion's draft (IMP-1). It runs
 *  once per suggestion and language for the session (each run is paid, so
 *  the result is kept); a failed or refused check is no check, so the review still opens. */
export function useImportDraftCheck(draft: Draft | undefined, lang: string) {
  return useQuery({
    queryKey: importKeys.check(draft?.source_url ?? "", lang),
    queryFn: () => checkImportDraft(draft!, lang),
    enabled: draft !== undefined,
    staleTime: Infinity,
    gcTime: Infinity,
    retry: false,
  })
}

export function useImportLineLookup(query: string, lang: string = "de") {
  return useQuery({
    queryKey: importKeys.lineLookup(query),
    queryFn: () => lookupImportLine(query, lang),
    enabled: query.trim().length >= 2,
    staleTime: 60_000,
  })
}

export function useResolveImport() {
  const { t } = useTranslation()
  return useMutation({
    mutationFn: (req: ResolveRequest) => resolveImport(req),
    onError: (err) => toastError(err, t),
  })
}
