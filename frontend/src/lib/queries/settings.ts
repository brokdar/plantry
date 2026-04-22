import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import {
  clearSetting,
  getSystemInfo,
  listAIModels,
  listSettings,
  setSetting,
} from "../api/settings"

import { aiKeys, settingsKeys } from "./keys"

export function useSettings() {
  return useQuery({
    queryKey: settingsKeys.list(),
    queryFn: listSettings,
    staleTime: 30_000,
  })
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
  return useMutation({
    mutationFn: ({ key, value }: { key: string; value: string }) =>
      setSetting(key, value),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: settingsKeys.all })
      qc.invalidateQueries({ queryKey: aiKeys.settings() })
    },
  })
}

export function useClearSetting() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (key: string) => clearSetting(key),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: settingsKeys.all })
      qc.invalidateQueries({ queryKey: aiKeys.settings() })
    },
  })
}

export function useAIModels(
  provider: string | undefined,
  apiKey: string | undefined,
  enabled: boolean
) {
  return useQuery({
    queryKey: [...settingsKeys.aiModels(provider ?? ""), apiKey ?? "stored"],
    queryFn: () => listAIModels(provider!, apiKey || undefined),
    enabled: enabled && !!provider,
    staleTime: 5 * 60_000,
    retry: false,
  })
}
