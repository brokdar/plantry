import { useMutation, useQuery } from "@tanstack/react-query"
import {
  extractRecipe,
  lookupImportLine,
  resolveImport,
  type ExtractRequest,
  type ResolveRequest,
} from "@/lib/api/import"
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
  return useMutation({
    mutationFn: (req: ResolveRequest) => resolveImport(req),
  })
}
