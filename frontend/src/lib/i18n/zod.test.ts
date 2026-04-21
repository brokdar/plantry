import { describe, it, expect, beforeAll } from "vitest"
import { z } from "zod"

import i18n from "./index"
import { configureZodI18n } from "./zod"

describe("configureZodI18n", () => {
  beforeAll(() => {
    configureZodI18n(i18n)
  })

  it("translates too_small on strings in English", async () => {
    await i18n.changeLanguage("en")
    const result = z.string().min(1).safeParse("")
    expect(result.success).toBe(false)
    if (!result.success) {
      expect(result.error.issues[0].message).toBe(
        "Must be at least 1 characters"
      )
    }
  })

  it("translates too_small on strings in German", async () => {
    await i18n.changeLanguage("de")
    const result = z.string().min(1).safeParse("")
    expect(result.success).toBe(false)
    if (!result.success) {
      expect(result.error.issues[0].message).toBe("Mindestens 1 Zeichen")
    }
  })

  it("translates invalid_type for missing required field in German", async () => {
    await i18n.changeLanguage("de")
    const result = z.object({ name: z.string() }).safeParse({})
    expect(result.success).toBe(false)
    if (!result.success) {
      expect(result.error.issues[0].message).toBe(
        "Dieses Feld ist erforderlich"
      )
    }
  })

  it("translates too_small on arrays in German", async () => {
    await i18n.changeLanguage("de")
    const result = z.array(z.string()).min(1).safeParse([])
    expect(result.success).toBe(false)
    if (!result.success) {
      expect(result.error.issues[0].message).toBe(
        "Mindestens 1 Eintrag hinzufügen"
      )
    }
  })
})
