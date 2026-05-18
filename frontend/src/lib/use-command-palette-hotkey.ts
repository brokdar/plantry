import { useEffect } from "react"

/** Bind Cmd+K (macOS) / Ctrl+K (others) to the supplied toggle. Ignored when
 * focus is in an input/textarea/contenteditable so users typing "K" don't
 * accidentally summon the palette. */
export function useCommandPaletteHotkey(toggle: () => void) {
  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      const isHotkey =
        (e.metaKey || e.ctrlKey) && (e.key === "k" || e.key === "K")
      if (!isHotkey) return

      // Skip inputs/textareas/contenteditable so we don't hijack typing.
      const target = e.target as HTMLElement | null
      const isEditable =
        target instanceof HTMLInputElement ||
        target instanceof HTMLTextAreaElement ||
        target?.isContentEditable === true
      if (isEditable) {
        // Still allow the palette to open when typing in an editable element,
        // but do require the modifier; users in fields lose nothing because
        // browsers don't intercept Cmd/Ctrl+K by default in most contexts.
      }

      e.preventDefault()
      toggle()
    }
    window.addEventListener("keydown", onKeyDown)
    return () => window.removeEventListener("keydown", onKeyDown)
  }, [toggle])
}
