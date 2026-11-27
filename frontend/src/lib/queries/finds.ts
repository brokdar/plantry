import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import {
  discardFind,
  getFind,
  keepFind,
  requestFind,
  type FindReason,
} from "@/lib/api/finds"
import { toastError } from "@/lib/toast"

import { foodKeys } from "./keys"

/** How often a searching find is polled. */
export const FIND_POLL_MS = 1000

export const findKeys = {
  detail: (id: number) => ["finds", id] as const,
}

/** A find; polls every second while it is searching. */
export function useFind(id: number | undefined) {
  return useQuery({
    queryKey: findKeys.detail(id ?? 0),
    queryFn: () => getFind(id!),
    enabled: id !== undefined,
    refetchInterval: (query) =>
      query.state.status !== "error" && query.state.data?.state === "searching"
        ? FIND_POLL_MS
        : false,
  })
}

export function useRequestFind() {
  const { t } = useTranslation()
  return useMutation({
    mutationFn: (dish: string) => requestFind(dish),
    onError: (err) => toastError(err, t),
  })
}

/** Links a find to the food saved from its review. */
export function useKeepFind() {
  const { t } = useTranslation()
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, foodId }: { id: number; foodId: number }) =>
      keepFind(id, foodId),
    onSuccess: (f) => {
      qc.setQueryData(findKeys.detail(f.id), f)
      void qc.invalidateQueries({ queryKey: foodKeys.lists() })
    },
    onError: (err) => toastError(err, t),
  })
}

export function useDiscardFind() {
  const { t } = useTranslation()
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, reason }: { id: number; reason: FindReason }) =>
      discardFind(id, reason),
    onSuccess: (f) => qc.setQueryData(findKeys.detail(f.id), f),
    onError: (err) => toastError(err, t),
  })
}
