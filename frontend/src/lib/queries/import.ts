import { useMutation, useQuery } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
import {
  extractRecipe,
  lookupImportLine,
  resolveImport,
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
