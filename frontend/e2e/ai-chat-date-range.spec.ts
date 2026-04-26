import { test, expect } from "@playwright/test"

import { API, apiRequest } from "./helpers"

function todayISO(): string {
  const d = new Date()
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`
}

function dateOffset(n: number): string {
  const d = new Date()
  d.setDate(d.getDate() + n)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`
}

interface PlateListResponse {
  items: { id: number; date: string; components?: unknown[] }[]
}

async function getPlatesForRange(
  from: string,
  to: string
): Promise<PlateListResponse> {
  const ctx = await apiRequest.newContext({ baseURL: API })
  const res = await ctx.get(`/api/plates?from=${from}&to=${to}`)
  const body = await res.json()
  await ctx.dispose()
  return body as PlateListResponse
}

test.describe("AI chat — date-range", () => {
  // Gate: skip unless AI_E2E=1 is set in the environment.
  // Run with: AI_E2E=1 bunx playwright test ai-chat-date-range
  test.skip(!process.env.AI_E2E, "AI_E2E not set — skipping AI e2e")

  test("agent uses dates not week ids when planning meals", async ({
    page,
  }) => {
    const from = todayISO()
    const to = dateOffset(6)

    await page.goto("/")

    // Open chat panel
    const openBtn = page.getByTestId("chat-open-button")
    await expect(openBtn).toBeVisible()
    await openBtn.click()

    const input = page.getByTestId("chat-composer-input")
    await expect(input).toBeVisible()
    await input.fill("Plan my next 7 days with vegetarian dinners.")

    // Wait for the AI chat response
    const chatResponse = page.waitForResponse(
      (r) => r.url().includes("/api/ai/chat") && r.request().method() === "POST"
    )
    await page.getByTestId("chat-composer-submit").click()
    await chatResponse

    // Wait for the assistant message to appear in the transcript
    await expect(
      page.getByTestId("chat-message-assistant").first()
    ).toBeVisible({ timeout: 30_000 })

    // Verify at least one plate was created in the next-7-day range via API
    const plates = await getPlatesForRange(from, to)
    expect(plates.items.length).toBeGreaterThanOrEqual(1)

    // At least one plate in the range should have a valid date string
    const datesInRange = plates.items.filter(
      (p) => p.date >= from && p.date <= to
    )
    expect(datesInRange.length).toBeGreaterThanOrEqual(1)
  })
})
