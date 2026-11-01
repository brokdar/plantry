import {
  cleanupSlot,
  expect,
  openPalette,
  seedSlot,
  test,
  uid,
} from "./helpers"

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

  // F10-AC3 (live part): the palette's go-to entries.
  test("Ctrl+K opens the palette, which goes to every destination", async ({
    page,
  }) => {
    await page.goto("/today")
    const palette = await openPalette(page)
    const goTo = palette.getByRole("group", { name: "Go to" })
    await expect(goTo.getByRole("option")).toHaveText([
      "Today",
      "Plan",
      "Recipes",
      "Ingredients",
      "Shop",
      "Settings",
    ])

    await palette.getByRole("combobox").fill("Shop")
    await page.keyboard.press("Enter")
    await expect(page).toHaveURL(/\/shop/)
    await expect(palette).toHaveCount(0)
  })

  test("Ctrl+Enter in the assistant composer submits the message", async ({
    page,
  }) => {
    // Serial-safe: we don't seed a conversation — we open the assistant from the
    // board's Ask assistant and rely on the existing fake-AI script
    // having turns available. If assistant turns are exhausted in shared state,
    // the fake provider short-circuits but the composer still clears on
    // submit, which is what we assert.
    const slot = await seedSlot(`slot.kb-${uid()}`, "Moon", 994)
    try {
      await page.setViewportSize({ width: 1440, height: 900 })
      await page.goto("/plan")
      await page.getByRole("button", { name: "Ask assistant" }).click()

      const composer = page.getByTestId("assistant-composer-input")
      await expect(composer).toBeVisible()
      await composer.fill("Plan my week")

      // Ctrl+Enter submits and clears the composer.
      const resp = page.waitForResponse(
        (r) =>
          r.url().includes("/api/ai/chat") && r.request().method() === "POST"
      )
      // Wait for the conversation list refetch that fires once the SSE
      // stream fully drains — keeps the fake AI client's turn cursor clean.
      const convRefetch = page.waitForResponse(
        (r) =>
          /\/api\/ai\/conversations(\?|$)/.test(r.url()) &&
          r.request().method() === "GET" &&
          r.ok()
      )
      await composer.press("Control+Enter")
      await resp
      await expect(composer).toHaveValue("")
      await convRefetch
    } finally {
      await cleanupSlot(slot.id)
    }
  })
})
