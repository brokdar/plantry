import {
  API,
  cleanupSlot,
  expect,
  mockSettingValues,
  seedSlot,
  test,
  uid,
} from "./helpers"

// The assistant panel on /plan (F2). The webServer starts the backend with the
// fake provider and plan-dinner.json, so /api/ai/chat streams a deterministic
// transcript: text + a tool call, then text. Serial: the fake client's turn
// cursor is process-global.
test.describe.configure({ mode: "serial" })

const DESKTOP = { width: 1440, height: 900 }
const PHONE = { width: 375, height: 800 }

test("opens as an aside beside the board and closes again", async ({
  page,
}) => {
  await page.setViewportSize(DESKTOP)
  await page.goto("/plan")
  const grid = page.getByRole("grid")
  await expect(grid).toBeVisible()
  const before = (await grid.boundingBox())!.width

  const ask = page.getByRole("button", { name: "Ask assistant" })
  await ask.click()
  const panel = page.getByRole("complementary", { name: "Assistant" })
  await expect(panel).toBeVisible()
  await expect(page).toHaveURL(/assistant=1/)
  // F2-AC1: no dialog, the board gave up ~380 px, a cell still opens.
  await expect(page.getByRole("dialog")).toHaveCount(0)
  await expect
    .poll(async () => before - (await grid.boundingBox())!.width)
    .toBeGreaterThan(330)
  await expect
    .poll(async () => before - (await grid.boundingBox())!.width)
    .toBeLessThan(430)
  await expect(page.getByTestId("assistant-composer-input")).toBeFocused()
  // F4-AC10: a plain conversation, without a mode select.
  await expect(panel.getByRole("combobox")).toHaveCount(0)

  await page.getByTestId("plan-cell").first().click()
  await expect(page.getByTestId("plan-sheet")).toBeVisible()
  await expect(panel).toBeVisible()
  await page.keyboard.press("Escape")
  await expect(page.getByTestId("plan-sheet")).toHaveCount(0)

  // Reload keeps it open (state is in the URL); Escape inside closes it and
  // focus returns to the toggle.
  await page.reload()
  await expect(panel).toBeVisible()
  await page.getByTestId("assistant-composer-input").focus()
  await page.keyboard.press("Escape")
  await expect(panel).toHaveCount(0)
  await expect(page).not.toHaveURL(/assistant=1/)
  await expect(ask).toBeFocused()
})

test("A toggles the panel, ignored in the composer and listed in ?", async ({
  page,
}) => {
  await page.setViewportSize(DESKTOP)
  await page.goto("/plan")
  const panel = page.getByRole("complementary", { name: "Assistant" })
  await expect(page.getByRole("grid")).toBeVisible()
  await expect(
    page.getByRole("button", { name: "Ask assistant" })
  ).not.toHaveAttribute("aria-disabled", "true")

  await page.getByRole("heading", { level: 1 }).click()
  await page.keyboard.press("a")
  await expect(panel).toBeVisible()

  // Typing an "a" in the composer is just a letter.
  const input = page.getByTestId("assistant-composer-input")
  await input.focus()
  await page.keyboard.press("a")
  await expect(input).toHaveValue("a")
  await expect(panel).toBeVisible()

  await page.getByRole("heading", { level: 1 }).click()
  await page.keyboard.press("a")
  await expect(panel).toHaveCount(0)

  await page.keyboard.press("?")
  const shortcuts = page.getByTestId("plan-shortcuts")
  await expect(shortcuts).toContainText("Open or close the assistant")
  // A dialog owns the keys.
  await page.keyboard.press("a")
  await expect(panel).toHaveCount(0)
})

test("without a provider the button is disabled with a tooltip and A does nothing", async ({
  page,
}) => {
  await mockSettingValues(page, { "ai.provider": "" })
  await page.setViewportSize(DESKTOP)
  await page.goto("/plan")
  const ask = page.getByRole("button", { name: "Ask assistant" })
  await expect(ask).toHaveAttribute("aria-disabled", "true")
  await ask.hover()
  await expect(page.getByRole("tooltip").first()).toContainText("AI provider")
  await page.getByRole("heading", { level: 1 }).click()
  await page.keyboard.press("a")
  await expect(page.getByTestId("assistant-panel")).toHaveCount(0)
})

test("on a phone it is a bottom sheet that a plate sheet replaces", async ({
  page,
}) => {
  await page.setViewportSize(PHONE)
  await page.goto("/plan")
  await page.getByRole("button", { name: "Ask assistant" }).click()
  const panel = page.getByRole("dialog", { name: "Assistant" })
  await expect(panel).toBeVisible()
  await expect(page.getByRole("complementary")).toHaveCount(0)
  await page.getByTestId("assistant-close").click()
  await expect(panel).toHaveCount(0)

  // Both asked for at once: the plate sheet wins.
  await page.goto("/plan")
  const key = await page
    .locator("[data-cell-key]")
    .first()
    .getAttribute("data-cell-key")
  await page.goto(`/plan?assistant=1&cell=${encodeURIComponent(key!)}`)
  await expect(page.getByTestId("plan-sheet")).toBeVisible()
  await expect(page.getByTestId("assistant-panel")).toHaveCount(0)
})

test("streams text and tool-call states, keeps history and starts new conversations", async ({
  page,
  request,
}) => {
  const slot = await seedSlot(`slot.assistant-${uid()}`, "moon", 99)
  try {
    await page.setViewportSize(DESKTOP)
    await page.goto("/plan")
    await page.getByRole("button", { name: "Ask assistant" }).click()

    // F2-AC5: a chip fills the composer and does not send.
    const input = page.getByTestId("assistant-composer-input")
    await page.getByRole("button", { name: "Lighter dinners" }).click()
    await expect(input).toHaveValue("Lighter dinners")
    await input.fill("plan my tuesday dinner")

    const chatResponse = page.waitForResponse(
      (r) => r.url().includes("/api/ai/chat") && r.request().method() === "POST"
    )
    const convRefetch = page.waitForResponse(
      (r) =>
        /\/api\/ai\/conversations(\?|$)/.test(r.url()) &&
        r.request().method() === "GET" &&
        r.ok()
    )
    await input.press("Control+Enter")
    await chatResponse
    await expect(input).toHaveValue("")

    await expect(page.getByTestId("assistant-tool-call").first()).toBeVisible({
      timeout: 5_000,
    })
    await expect(
      page.getByTestId("assistant-message-assistant").first()
    ).toBeVisible({ timeout: 10_000 })
    await convRefetch
    // Chips are for an empty conversation only.
    await expect(page.getByTestId("assistant-chips")).toHaveCount(0)

    const conv = await request.get(`${API}/api/ai/conversations`)
    expect(((await conv.json()) as { total: number }).total).toBeGreaterThan(0)

    await page.getByRole("button", { name: /open history/i }).click()
    await expect(
      page.getByRole("button", { name: /open conversation/i }).first()
    ).toBeVisible()
    await page.keyboard.press("Escape")

    await page.getByTestId("assistant-new-conversation").click()
    await expect(page.getByTestId("assistant-new-conversation")).toHaveCount(0)
    await expect(input).toBeVisible()
    await expect(page.getByTestId("assistant-chips")).toBeVisible()
  } finally {
    await cleanupSlot(slot.id)
  }
})
