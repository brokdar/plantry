import {
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient,
} from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import {
  clearSetting,
  getSystemInfo,
  listSettings,
  setSetting,
  type SettingItem,
  type SettingsList,
} from "../api/settings"

import { PRICE_OVERRIDES_KEY } from "@/lib/api/prices"
import { toastError } from "@/lib/toast"

import { aiKeys, insightKeys, runKeys, settingsKeys } from "./keys"

/** The settings list indexed by key. */
export function useSettingItems() {
  return useQuery({
    queryKey: settingsKeys.list(),
    queryFn: listSettings,
    staleTime: 30_000,
    select: indexSettings,
  })
}

function indexSettings(data: SettingsList): Record<string, SettingItem> {
  return Object.fromEntries(data.items.map((it) => [it.key, it]))
}

export function useSystemInfo() {
  return useQuery({
    queryKey: settingsKeys.system(),
    queryFn: getSystemInfo,
    staleTime: 60_000,
  })
}

export function useSetSetting() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  return useMutation({
    mutationFn: ({ key, value }: { key: string; value: string }) =>
      setSetting(key, value),
    // Show the new value at once; the invalidation below confirms it. The
    // cache write happens before the first await, so a controlled input
    // re-renders with the new value in the same event (no flicker back).
    onMutate: async ({ key, value }) => {
      const cancelled = qc.cancelQueries({ queryKey: settingsKeys.list() })
      qc.setQueryData<SettingsList>(settingsKeys.list(), (prev) =>
        prev
          ? {
              ...prev,
              items: prev.items.map((it) =>
                it.key === key && !it.is_secret
                  ? { ...it, value, source: "db" }
                  : it
              ),
            }
          : prev
      )
      await cancelled
    },
    onSuccess: (_, { key }) => {
      qc.invalidateQueries({ queryKey: settingsKeys.all })
      qc.invalidateQueries({ queryKey: aiKeys.settings() })
      invalidatePrices(qc, key)
    },
    onError: (err) => {
      toastError(err, t)
      qc.invalidateQueries({ queryKey: settingsKeys.list() })
    },
  })
}

export function useClearSetting() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  return useMutation({
    mutationFn: (key: string) => clearSetting(key),
    onSuccess: (_, key) => {
      qc.invalidateQueries({ queryKey: settingsKeys.all })
      qc.invalidateQueries({ queryKey: aiKeys.settings() })
      invalidatePrices(qc, key)
    },
    onError: (err) => toastError(err, t),
  })
}

/** A price override re-prices runs on read: drop the cached costs. */
function invalidatePrices(qc: QueryClient, key: string) {
  if (key !== PRICE_OVERRIDES_KEY) return
  qc.invalidateQueries({ queryKey: runKeys.all })
  qc.invalidateQueries({ queryKey: insightKeys.all })
}
