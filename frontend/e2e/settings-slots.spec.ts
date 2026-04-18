import { apiRequest, expect, test } from "./helpers"

const API = "http://localhost:8080"

function uid() {
  return crypto.randomUUID().slice(0, 8)
}

async function cleanupSlot(id: number) {
  const ctx = await apiRequest.newContext({ baseURL: API })
  await ctx.delete(`/api/settings/slots/${id}`)
  await ctx.dispose()
}

async function listSlots() {
  const ctx = await apiRequest.newContext({ baseURL: API })
  const res = await ctx.get(`/api/settings/slots`)
  const body = (await res.json()) as {
    items: { id: number; name_key: string }[]
  }
  await ctx.dispose()
  return body.items
}

test.describe("Time slots settings", () => {
  test("create, list, delete a time slot via the UI", async ({ page }) => {
    const tag = uid()
    const nameKey = `slot.test_${tag}`
    let createdId: number | undefined

    try {
      await page.goto("/settings")

      await page.getByLabel(/translation key/i).fill(nameKey)
      await page.getByLabel(/^icon$/i).fill("Coffee")
      await page.getByLabel(/order/i).fill("99")

      const createResp = page.waitForResponse(
        (r) =>
          r.url().includes("/api/settings/slots") &&
          r.request().method() === "POST"
      )
      await page.getByRole("button", { name: "Save", exact: true }).click()
      const response = await createResp
      expect(response.status()).toBe(201)
      const created = (await response.json()) as { id: number }
      createdId = created.id

      // Newly created slot row appears.
      await expect(
        page.locator(`[data-testid="slot-row-${createdId}"]`)
      ).toBeVisible()

      // Delete via the row's button — failOnDialog auto-accepts confirms.
      const deleteResp = page.waitForResponse(
        (r) =>
          r.url().includes(`/api/settings/slots/${createdId}`) &&
          r.request().method() === "DELETE"
      )
      await page
        .locator(`[data-testid="slot-row-${createdId}"]`)
        .getByRole("button", { name: /delete/i })
        .click()
      await deleteResp

      await expect(page.getByText(nameKey)).toHaveCount(0)
      createdId = undefined
    } finally {
      if (createdId) await cleanupSlot(createdId)
    }
  })

  test("validation surfaces when name_key is empty", async ({ page }) => {
    await page.goto("/settings")
    await page.getByLabel(/^icon$/i).fill("Coffee")
    await page.getByRole("button", { name: "Save", exact: true }).click()
    // UI should show the validation error inline.
    await expect(page.getByText("name_key required")).toBeVisible()
    // Form should not POST; confirm no slot was created.
    const slots = await listSlots()
    expect(slots.find((s) => s.name_key === "")).toBeUndefined()
  })
})
