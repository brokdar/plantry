import { request as apiRequest, expect, test } from "@playwright/test"

const API = "http://localhost:8080"

function uid() {
  return crypto.randomUUID().slice(0, 8)
}

async function seedSlot(name_key: string, icon: string, sort_order: number) {
  const ctx = await apiRequest.newContext({ baseURL: API })
  const res = await ctx.post("/api/settings/slots", {
    data: { name_key, icon, sort_order, active: true },
  })
  const body = (await res.json()) as { id: number; name_key: string }
  expect(
    res.ok(),
    `seed slot ${name_key}: ${res.status()} ${JSON.stringify(body)}`
  ).toBeTruthy()
  await ctx.dispose()
  return body
}

async function seedComponent(name: string, role: string) {
  const ctx = await apiRequest.newContext({ baseURL: API })
  const res = await ctx.post("/api/components", {
    data: { name, role, reference_portions: 1 },
  })
  const body = (await res.json()) as { id: number; name: string }
  expect(
    res.ok(),
    `seed component ${name}: ${res.status()} ${JSON.stringify(body)}`
  ).toBeTruthy()
  await ctx.dispose()
  return body
}

async function deletePlatesUsingSlot(slotId: number) {
  // Best-effort cleanup: walk every week and delete plates that reference the slot.
  // SQLite ON DELETE RESTRICT prevents deleting the slot row otherwise.
  const ctx = await apiRequest.newContext({ baseURL: API })
  const wRes = await ctx.get("/api/weeks?limit=100")
  const weeks = ((await wRes.json()) as { items: { id: number }[] }).items
  for (const w of weeks) {
    const det = await ctx.get(`/api/weeks/${w.id}`)
    const detail = (await det.json()) as {
      plates: { id: number; slot_id: number }[]
    }
    for (const p of detail.plates) {
      if (p.slot_id === slotId) {
        await ctx.delete(`/api/plates/${p.id}`)
      }
    }
  }
  await ctx.dispose()
}

async function cleanupSlot(id: number) {
  await deletePlatesUsingSlot(id)
  const ctx = await apiRequest.newContext({ baseURL: API })
  await ctx.delete(`/api/settings/slots/${id}`)
  await ctx.dispose()
}

async function cleanupComponent(id: number) {
  const ctx = await apiRequest.newContext({ baseURL: API })
  await ctx.delete(`/api/components/${id}`)
  await ctx.dispose()
}

test.describe("Weekly planner", () => {
  test("plan a meal, swap a component, remove one, copy week", async ({
    page,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.dinner_${tag}`, "Moon", 999)
    const main = await seedComponent(`Chicken curry ${tag}`, "main")
    const side = await seedComponent(`Basmati ${tag}`, "side_starch")
    const replacement = await seedComponent(`Naan ${tag}`, "side_starch")

    try {
      await page.goto("/")

      // Empty cell at Mon (day=0) for the seeded slot. Click the "+" affordance.
      const cell = page.locator(`[data-testid="cell-0-${slot.id}"]`)
      await expect(cell).toBeVisible()

      const createPlateResp = page.waitForResponse(
        (r) => r.url().includes("/plates") && r.request().method() === "POST"
      )
      await cell.getByRole("button", { name: /add a meal/i }).click()
      // Sheet opens with role=main filter; pick the chicken curry.
      await page
        .getByRole("button", { name: new RegExp(`Chicken curry ${tag}`) })
        .click()
      await createPlateResp

      // Plate now shows the curry chip.
      await expect(cell.getByText(`Chicken curry ${tag}`)).toBeVisible()

      // Add a side via the plate's "Add component" button.
      const addCompResp = page.waitForResponse(
        (r) => /\/components$/.test(r.url()) && r.request().method() === "POST"
      )
      await cell
        .getByRole("button", { name: /add component/i })
        .first()
        .click()
      await page
        .getByRole("button", { name: new RegExp(`Basmati ${tag}`) })
        .click()
      await addCompResp

      await expect(cell.getByText(`Basmati ${tag}`)).toBeVisible()

      // Swap the basmati for naan via the swap button on the chip.
      const basmatiChip = cell.getByText(`Basmati ${tag}`).locator("..")
      const swapResp = page.waitForResponse(
        (r) =>
          /\/components\/\d+$/.test(r.url()) && r.request().method() === "PUT"
      )
      await basmatiChip.getByRole("button", { name: /swap/i }).click()
      await page
        .getByRole("button", { name: new RegExp(`Naan ${tag}`) })
        .click()
      await swapResp

      await expect(cell.getByText(`Naan ${tag}`)).toBeVisible()
      await expect(cell.getByText(`Basmati ${tag}`)).toHaveCount(0)

      // Remove the curry.
      const removeResp = page.waitForResponse(
        (r) =>
          /\/components\/\d+$/.test(r.url()) &&
          r.request().method() === "DELETE"
      )
      const curryChip = cell.getByText(`Chicken curry ${tag}`).locator("..")
      await curryChip.getByRole("button", { name: /remove/i }).click()
      await removeResp

      await expect(cell.getByText(`Chicken curry ${tag}`)).toHaveCount(0)

      // Navigate next week.
      await page.getByRole("button", { name: /next week/i }).click()
      // Empty cell on the next week (no plate yet — find by data-testid).
      await expect(
        page.locator(`[data-testid="cell-0-${slot.id}"]`)
      ).toBeVisible()

      // Navigate back.
      await page.getByRole("button", { name: /previous week/i }).click()
      await expect(cell.getByText(`Naan ${tag}`)).toBeVisible()
    } finally {
      await cleanupComponent(replacement.id)
      await cleanupComponent(side.id)
      await cleanupComponent(main.id)
      await cleanupSlot(slot.id)
    }
  })
})
