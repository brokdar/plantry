import i18next from "i18next"
import { describe, expect, it } from "vitest"

import "@/lib/i18n"
import de from "@/lib/i18n/de.json"
import en from "@/lib/i18n/en.json"
import { mockChange } from "@/test/fixtures"

import { CHANGE_TEXT_KEYS, changeSentence, statusKey } from "./change-text"

type Copy = Record<string, string>

/** A key exists as itself or as a plural form. */
function has(copy: Copy, key: string): boolean {
  return key in copy || `${key}_one` in copy || `${key}_other` in copy
}

describe("change text", () => {
  it("every text_key has en and de copy", () => {
    for (const [lang, copy] of [
      ["en", en.assistant.change],
      ["de", de.assistant.change],
    ] as const) {
      for (const key of CHANGE_TEXT_KEYS) {
        expect(has(copy as Copy, key), `${lang}: ${key}`).toBe(true)
      }
    }
  })

  it("renders the portions line with the frozen part only when frozen", () => {
    const t = i18next.t.bind(i18next)
    const line = (frozen: number) =>
      changeSentence(
        t,
        i18next,
        mockChange({ params: { name: "Chili", portions: 6, frozen } })
      )
    expect(line(2)).toBe("Chili: 6 portions, 2 frozen")
    expect(line(0)).toBe("Chili: 6 portions")
  })

  it("renders dates as weekdays and slots by name", () => {
    const t = i18next.t.bind(i18next)
    expect(
      changeSentence(
        t,
        i18next,
        mockChange({
          text_key: "set_availability_none",
          params: { date: "2026-11-03", slot: "slot.dinner", mode: "none" },
        })
      )
    ).toBe("Tuesday dinner: no plate")
    expect(
      changeSentence(
        t,
        i18next,
        mockChange({
          text_key: "regenerate_side",
          params: { date: "2026-11-03", name: "Rice", count: 1 },
        })
      )
    ).toBe("Tuesday: new side instead of Rice")
  })

  it("falls back to a plain line for an unknown key", () => {
    const t = i18next.t.bind(i18next)
    expect(
      changeSentence(t, i18next, mockChange({ text_key: "nope", params: {} }))
    ).toBe("Your plan changed")
  })

  it("status keys never carry a tool name", () => {
    expect(statusKey("get_plan")).toBe("assistant.status.reading")
    expect(statusKey("regenerate")).toBe("assistant.status.regenerating")
    expect(statusKey("set_session_portions")).toBe("assistant.status.updating")
  })
})
