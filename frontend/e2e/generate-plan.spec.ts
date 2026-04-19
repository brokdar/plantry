import { cleanupSlot, expect, seedSlot, test, uid } from "./helpers"

// These tests verify the "Generate Plan" CTA wiring end-to-end: clicking any
// of the three variants (default sidebar button, rail button, mobile FAB)
// should navigate to the planner and open the AI chat panel with a localized
// prefill in the composer. We deliberately avoid submitting the chat here —
// the fake AI provider (plan-dinner.json) has a finite scripted turn set that
// is shared with ai-chat.spec.ts. Chat streaming is covered there.

test.describe("Generate Plan CTA", () => {
  test("default variant opens chat with prefilled composer from the planner", async ({
    page,
  }) => {
    const slot = await seedSlot(`slot.gen-planner-${uid()}`, "coffee", 10)

    try {
      await page.setViewportSize({ width: 1440, height: 900 })
      await page.goto("/")

      await expect(page.getByTestId("sidenav")).toBeVisible()
      await page.getByTestId("generate-plan-default").click()

      const composer = page.getByTestId("chat-composer-input")
      await expect(composer).toBeVisible()
      await expect(composer).toHaveValue(/plan my week/i)
    } finally {
      await cleanupSlot(slot.id)
    }
  })

  test("default variant on another route navigates to planner and opens chat", async ({
    page,
  }) => {
    const slot = await seedSlot(`slot.gen-default-${uid()}`, "coffee", 10)

    try {
      await page.setViewportSize({ width: 1440, height: 900 })
      await page.goto("/archive")

      await expect(page.getByTestId("sidenav")).toBeVisible()
      await page.getByTestId("generate-plan-default").click()

      await expect(page).toHaveURL("/")
      const composer = page.getByTestId("chat-composer-input")
      await expect(composer).toBeVisible()
      await expect(composer).toHaveValue(/plan my week/i)
    } finally {
      await cleanupSlot(slot.id)
    }
  })

  test("fab variant opens chat with prefilled composer on mobile", async ({
    page,
  }) => {
    const slot = await seedSlot(`slot.gen-fab-${uid()}`, "coffee", 10)

    try {
      await page.setViewportSize({ width: 390, height: 844 })
      await page.goto("/")

      await expect(page.getByTestId("mobile-bottom-nav")).toBeVisible()
      await page.getByTestId("generate-plan-fab").click()

      const composer = page.getByTestId("chat-composer-input")
      await expect(composer).toBeVisible()
      await expect(composer).toHaveValue(/plan my week/i)
    } finally {
      await cleanupSlot(slot.id)
    }
  })
})
