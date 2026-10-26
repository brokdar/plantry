import { useCallback, useEffect, useRef, useState } from "react"

import type { SettingItem } from "@/lib/api/settings"
import {
  useClearSetting,
  useSetSetting,
  useSettingItems,
} from "@/lib/queries/settings"

const SAVED_NOTE_MS = 2500

/** A short-lived flag for the quiet "Saved" note next to a setting. */
export function useSavedNote() {
  const [saved, setSaved] = useState(false)
  const timer = useRef<number | undefined>(undefined)
  useEffect(() => () => window.clearTimeout(timer.current), [])
  const markSaved = useCallback(() => {
    setSaved(true)
    window.clearTimeout(timer.current)
    timer.current = window.setTimeout(() => setSaved(false), SAVED_NOTE_MS)
  }, [])
  return { saved, markSaved }
}

export interface SettingField {
  item: SettingItem | undefined
  /** The stored value, or `fallback` while the list loads or lacks the key. */
  value: string
  save: (value: string) => void
  clear: (() => void) | undefined
  saved: boolean
}

/** One `/api/settings` key: its value, save-on-change and clear override. */
export function useSettingField(key: string, fallback: string): SettingField {
  const { data } = useSettingItems()
  const setSetting = useSetSetting()
  const clearSetting = useClearSetting()
  const { saved, markSaved } = useSavedNote()
  const item = data?.[key]

  return {
    item,
    value: item?.value || fallback,
    save: (value) =>
      setSetting.mutate({ key, value }, { onSuccess: markSaved }),
    clear:
      item?.source === "db"
        ? () => clearSetting.mutate(key, { onSuccess: markSaved })
        : undefined,
    saved,
  }
}
