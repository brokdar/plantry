import { cleanupSlot, expect, seedSlot, test, uid } from "./helpers"

// Additional AI-chat coverage beyond ai-chat.spec.ts: opening the history
// popover lists the persisted conversations, and the "New conversation"
// button clears the active transcript. Serial so the fake AI client's
// scripted-turn state is predictable (same rationale as ai-chat.spec.ts).
test.describe.configure({ mode: "serial" })

test.describe("AI chat — history + new conversation", () => {
  test("history popover lists past conversations after a turn", async ({
    page,
  }) => {
    const slot = await seedSlot(`slot.chat-hist-${uid()}`, "Moon", 993)

    try {
      await page.goto("/")

      await page.getByTestId("chat-open-button").click()
      await page.getByTestId("chat-composer-input").fill("hello")

      const resp = page.waitForResponse(
        (r) =>
          r.url().includes("/api/ai/chat") && r.request().method() === "POST"
      )
      await page.getByTestId("chat-composer-submit").click()
      await resp

      // History button reveals the popover.
      await page.getByRole("button", { name: /open history/i }).click()
      // At least one conversation entry is present.
      await expect(
        page.getByRole("button", { name: /open conversation/i }).first()
      ).toBeVisible()
    } finally {
      await cleanupSlot(slot.id)
    }
  })

  test("'New conversation' button clears the active thread", async ({
    page,
  }) => {
    const slot = await seedSlot(`slot.chat-new-${uid()}`, "Moon", 992)

    try {
      await page.goto("/")

      await page.getByTestId("chat-open-button").click()
      await page.getByTestId("chat-composer-input").fill("hi there")

      const resp = page.waitForResponse(
        (r) =>
          r.url().includes("/api/ai/chat") && r.request().method() === "POST"
      )
      await page.getByTestId("chat-composer-submit").click()
      await resp

      // 'New conversation' button appears once a conversation is active.
      const newBtn = page.getByTestId("chat-new-conversation")
      await expect(newBtn).toBeVisible()
      await newBtn.click()

      // Composer remains and button disappears — an active conversation
      // is required to show the new-conversation affordance.
      await expect(page.getByTestId("chat-new-conversation")).toHaveCount(0)
      await expect(page.getByTestId("chat-composer-input")).toBeVisible()
    } finally {
      await cleanupSlot(slot.id)
    }
  })
})
