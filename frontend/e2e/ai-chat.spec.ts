import { expect, test } from "./helpers"

import { API, cleanupSlot, seedSlot, uid } from "./helpers"

// The webServer starts the backend with PLANTRY_AI_PROVIDER=fake and the
// plan-dinner.json script, so /api/ai/chat streams a deterministic two-turn
// transcript: text + list_slots tool call → text. Tests run serially to keep
// the fake client's scripted-turn state predictable across chat sessions.
test.describe.configure({ mode: "serial" })

test("chat panel streams assistant text and tool-call states", async ({
  page,
  request,
}) => {
  const slot = await seedSlot(`slot.chat-${uid()}`, "moon", 99)

  try {
    await page.goto("/")

    const openBtn = page.getByTestId("chat-open-button")
    await expect(openBtn).toBeVisible()
    await openBtn.click()

    const input = page.getByTestId("chat-composer-input")
    await expect(input).toBeVisible()
    await input.fill("plan my tuesday dinner")

    const chatResponse = page.waitForResponse(
      (r) => r.url().includes("/api/ai/chat") && r.request().method() === "POST"
    )
    await page.getByTestId("chat-composer-submit").click()
    await chatResponse

    // Tool card appears during streaming. Match running or ok — either
    // confirms the tool-call streaming pipeline end-to-end. Multiple may
    // render across the streaming transcript and conversation refetch — we
    // only need at least one to prove the tool-call pipeline is alive.
    const toolCard = page.getByTestId("chat-tool-call").first()
    await expect(toolCard).toBeVisible({ timeout: 5_000 })

    // Final assistant message shows up in transcript (persisted conversation)
    // after stream closes and the conversation query refetches.
    await expect(
      page.getByTestId("chat-message-assistant").first()
    ).toBeVisible({ timeout: 10_000 })

    // Conversation persisted on backend.
    const conv = await request.get(`${API}/api/ai/conversations`)
    const body = (await conv.json()) as { total: number }
    expect(body.total).toBeGreaterThanOrEqual(1)
  } finally {
    await cleanupSlot(slot.id)
  }
})

test("chat composer submits on Ctrl+Enter", async ({ page }) => {
  const slot = await seedSlot(`slot.chat-${uid()}`, "moon", 99)

  try {
    await page.goto("/")
    await page.getByTestId("chat-open-button").click()

    const input = page.getByTestId("chat-composer-input")
    await input.fill("hi")

    const chatResponse = page.waitForResponse(
      (r) => r.url().includes("/api/ai/chat") && r.request().method() === "POST"
    )
    await input.press("Control+Enter")
    await chatResponse

    // After submit, a response arrived — confirm persisted transcript renders
    // at least one assistant message.
    await expect(
      page.getByTestId("chat-message-assistant").first()
    ).toBeVisible({ timeout: 10_000 })
  } finally {
    await cleanupSlot(slot.id)
  }
})
