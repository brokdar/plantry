import { cleanupSlot, expect, seedSlot, test, uid } from "./helpers"

test.describe("Keyboard shortcuts", () => {
  test("pressing 'd' toggles the theme", async ({ page }) => {
    await page.goto("/")

    const html = page.locator("html")
    const initiallyDark = ((await html.getAttribute("class")) ?? "").includes(
      "dark"
    )

    await page.keyboard.press("d")

    // Web-first assertion auto-retries until the class attribute flips.
    if (initiallyDark) {
      await expect(html).not.toHaveClass(/(?:^| )dark(?: |$)/)
    } else {
      await expect(html).toHaveClass(/(?:^| )dark(?: |$)/)
    }
  })

  test("Ctrl+Enter in chat composer submits the prefilled message", async ({
    page,
  }) => {
    // Serial-safe: we don't seed a conversation — we open chat via the
    // Generate Plan FAB on mobile and rely on the existing fake-AI script
    // having turns available. If chat turns are exhausted in shared state,
    // the fake provider short-circuits but the composer still clears on
    // submit, which is what we assert.
    const slot = await seedSlot(`slot.kb-${uid()}`, "Moon", 994)
    try {
      await page.goto("/")
      // On desktop the default variant renders a sidebar button.
      await page.setViewportSize({ width: 1440, height: 900 })
      await page.getByTestId("generate-plan-rail").click()

      const composer = page.getByTestId("chat-composer-input")
      await expect(composer).toBeVisible()
      await expect(composer).toHaveValue(/plan my week/i)

      // Ctrl+Enter submits and clears the composer.
      const resp = page.waitForResponse(
        (r) =>
          r.url().includes("/api/ai/chat") && r.request().method() === "POST"
      )
      await composer.press("Control+Enter")
      await resp
      await expect(composer).toHaveValue("")
    } finally {
      await cleanupSlot(slot.id)
    }
  })
})
