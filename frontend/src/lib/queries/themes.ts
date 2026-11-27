import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import {
  acceptThemeSuggestion,
  declineThemeSuggestion,
  getWindowThemes,
  removeWindowTheme,
  setWindowTheme,
  type ThemeKind,
  type ThemeResult,
} from "@/lib/api/themes"
import { toast, toastError } from "@/lib/toast"

import { planKeys } from "./keys"

/** Under planKeys.all: the assistant list counts the suggestion (PRO-4). */
export const themeKeys = {
  window: (start: string) => [...planKeys.all, "themes", start] as const,
}

export function useWindowThemes(start: string, enabled: boolean) {
  return useQuery({
    queryKey: themeKeys.window(start),
    queryFn: () => getWindowThemes(start),
    enabled,
  })
}

/** Theme mutations of one window; a set theme says what the library holds
 *  and what the assistant looks for ("No Japanese recipes yet. I'll find
 *  2."). */
export function useThemeActions(start: string) {
  const qc = useQueryClient()
  const { t } = useTranslation()
  const refresh = () => qc.invalidateQueries({ queryKey: planKeys.all })
  const said = (r: ThemeResult) => {
    const name = themeLabel(r.theme.value)
    if (r.finding > 0) {
      toast.success(
        r.library === 0
          ? t("plan.themes.finding_none", { name, count: r.finding })
          : t("plan.themes.finding_some", {
              name,
              count: r.finding,
              library: r.library,
            })
      )
    } else {
      toast.success(t("plan.themes.set", { name }))
    }
    return refresh()
  }
  const onError = (err: unknown) => toastError(err, t)
  return {
    set: useMutation({
      mutationFn: (input: {
        kind: ThemeKind
        value: string
        source?: "user" | "again"
      }) => setWindowTheme(start, input),
      onSuccess: said,
      onError,
    }),
    remove: useMutation({
      mutationFn: (id: number) => removeWindowTheme(start, id),
      onSuccess: refresh,
      onError,
    }),
    accept: useMutation({
      mutationFn: () => acceptThemeSuggestion(start),
      onSuccess: said,
      onError,
    }),
    decline: useMutation({
      mutationFn: () => declineThemeSuggestion(start),
      onSuccess: refresh,
      onError,
    }),
  }
}

/** A theme value as shown: first letter upper case ("japanese" ->
 *  "Japanese"). */
export function themeLabel(value: string): string {
  return value.charAt(0).toUpperCase() + value.slice(1)
}
