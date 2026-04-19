import { apiFetch } from "./client"

export type SettingSource = "db" | "env" | "default"

export interface SettingItem {
  key: string
  value?: string
  source: SettingSource
  is_secret: boolean
  masked_preview?: string
  env_also_set: boolean
}

export interface SettingsList {
  items: SettingItem[]
  cipher_available: boolean
}

export interface SystemInfo {
  port: number
  db_path: string
  log_level: string
  image_path: string
  dev_mode: boolean
  version: string
  build_commit: string
  cipher_available: boolean
}

export interface AIModel {
  id: string
  display_name?: string
}

export interface AIModelList {
  models: AIModel[]
  validated: boolean
}

export function listSettings() {
  return apiFetch<SettingsList>("/settings")
}

export function setSetting(key: string, value: string) {
  return apiFetch<void>(`/settings/${encodeURIComponent(key)}`, {
    method: "PUT",
    body: JSON.stringify({ value }),
  })
}

export function clearSetting(key: string) {
  return apiFetch<void>(`/settings/${encodeURIComponent(key)}`, {
    method: "DELETE",
  })
}

export function getSystemInfo() {
  return apiFetch<SystemInfo>("/settings/system")
}

export function listAIModels(provider: string, apiKey?: string) {
  const params = new URLSearchParams({ provider })
  if (apiKey) params.set("api_key", apiKey)
  return apiFetch<AIModelList>(`/settings/ai/models?${params.toString()}`)
}
