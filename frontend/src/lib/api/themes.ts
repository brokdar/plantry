import { apiFetch } from "./client"

/** A week theme's kind (THEME-2). */
export type ThemeKind = "cuisine" | "ingredient" | "mood"

export interface WeekTheme {
  id: number
  kind: ThemeKind
  value: string
  source: "user" | "chat" | "suggestion" | "again"
  whole_week: boolean
}

export interface ThemeSuggestion {
  kind: ThemeKind
  value: string
  state: "open" | "accepted" | "declined"
}

/** GET /api/windows/{start}/themes: the window's themes, the window
 *  before's to offer again (THEME-5) and the open suggestion (THEME-4). */
export interface WindowThemes {
  themes: WeekTheme[]
  again: WeekTheme[]
  suggestion: ThemeSuggestion | null
}

/** A set theme: library recipes that fit it and the finds it started. */
export interface ThemeResult {
  theme: WeekTheme
  library: number
  finding: number
}

const base = (start: string) => `/windows/${encodeURIComponent(start)}/themes`

export function getWindowThemes(start: string): Promise<WindowThemes> {
  return apiFetch(base(start))
}

export function setWindowTheme(
  start: string,
  input: { kind: ThemeKind; value: string; source?: "user" | "again" }
): Promise<ThemeResult> {
  return apiFetch(base(start), { method: "POST", body: JSON.stringify(input) })
}

export function removeWindowTheme(start: string, id: number): Promise<void> {
  return apiFetch(`${base(start)}/${id}`, { method: "DELETE" })
}

export function acceptThemeSuggestion(start: string): Promise<ThemeResult> {
  return apiFetch(`${base(start)}/suggestion/accept`, { method: "POST" })
}

export function declineThemeSuggestion(start: string): Promise<void> {
  return apiFetch(`${base(start)}/suggestion/decline`, { method: "POST" })
}
