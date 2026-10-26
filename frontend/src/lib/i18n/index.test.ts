import i18n from "./index"

describe("i18n document language", () => {
  afterEach(async () => {
    await i18n.changeLanguage("en")
  })

  it("sets <html lang> on init", () => {
    expect(document.documentElement.lang).toBe(i18n.resolvedLanguage)
  })

  it("updates <html lang> when the language changes", async () => {
    await i18n.changeLanguage("de")
    expect(document.documentElement.lang).toBe("de")
  })
})
