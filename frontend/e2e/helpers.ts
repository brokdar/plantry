import {
  request as apiRequest,
  expect,
  test as baseTest,
  type Locator,
  type Page,
} from "@playwright/test"

export const API = `http://localhost:${process.env.E2E_API_PORT ?? 8080}`

/**
 * Extended `test` that auto-installs the dialog guard on every page.
 * Import as: `import { test, expect } from "./helpers"`.
 */
export const test = baseTest.extend({
  page: async ({ page }, runTest) => {
    failOnDialog(page)
    await runTest(page)
  },
})

export { expect, apiRequest }

export function uid() {
  return crypto.randomUUID().slice(0, 8)
}

/**
 * Registers a dialog handler that fails the test on window.alert (we ship
 * toasts now, so any alert is a regression) while auto-accepting confirm
 * and auto-dismissing prompt so delete-flows continue to work.
 */
export function failOnDialog(page: Page) {
  page.on("dialog", (dialog) => {
    if (dialog.type() === "alert") {
      void dialog.dismiss()
      throw new Error(
        `Unexpected window.alert during test: "${dialog.message()}" — migrate this error path to toast.`
      )
    }
    if (dialog.type() === "prompt") {
      void dialog.dismiss()
      return
    }
    void dialog.accept()
  })
}

// ── Food seeding ──────────────────────────────────────────────────────
//
// The unified Food aggregate replaces the old Ingredient + Component split.
// `kind: "leaf"` foods carry per-100g nutrition directly (the old
// "Ingredient"). `kind: "composed"` foods reference child foods (the old
// "Component"). Plates and templates always reference a Food by id, regardless
// of kind, so leaf foods can be placed directly on a plate.

export interface SeedLeafInput {
  name: string
  kcal_100g?: number
  protein_100g?: number
  fat_100g?: number
  carbs_100g?: number
  fiber_100g?: number
  sodium_100g?: number
}

export interface SeedComposedChild {
  child_id: number
  amount: number
  unit: string
  grams: number
  sort_order: number
}

export interface SeedComposedInput {
  name: string
  role: string
  reference_portions?: number
  /** Days a cooked batch keeps (1–7); null uses the plan default. */
  keep_days?: number
  children?: SeedComposedChild[]
  instructions?: { step_number: number; text: string }[]
  tags?: string[]
}

export async function seedLeafFood(data: SeedLeafInput) {
  const ctx = await apiRequest.newContext({ baseURL: API })
  const res = await ctx.post("/api/foods", {
    data: { kind: "leaf", source: "manual", ...data },
  })
  const body = await res.json()
  expect(
    res.ok(),
    `Seed leaf food "${data.name}" failed: ${res.status()} ${JSON.stringify(body)}`
  ).toBeTruthy()
  await ctx.dispose()
  return body as { id: number; name: string; kind: "leaf" }
}

export async function seedComposedFood(data: SeedComposedInput) {
  const ctx = await apiRequest.newContext({ baseURL: API })
  const res = await ctx.post("/api/foods", {
    data: { kind: "composed", reference_portions: 1, ...data },
  })
  const body = await res.json()
  expect(
    res.ok(),
    `Seed composed food "${data.name}" failed: ${res.status()} ${JSON.stringify(body)}`
  ).toBeTruthy()
  await ctx.dispose()
  return body as { id: number; name: string; kind: "composed" }
}

export async function cleanupFood(id: number) {
  const ctx = await apiRequest.newContext({ baseURL: API })
  await ctx.delete(`/api/foods/${id}`)
  await ctx.dispose()
}

/**
 * Seeds a composed food that satisfies the "at least one child" invariant by
 * creating a throwaway leaf stub automatically. Returns both the composed food
 * and the stub so callers can clean up both.
 */
export async function seedComposedWithStub(
  data: SeedComposedInput,
  tag: string
) {
  const stub = await seedLeafFood({ name: `Stub ${tag}-${uid()}` })
  const composed = await seedComposedFood({
    ...data,
    children: data.children ?? [
      { child_id: stub.id, amount: 100, unit: "g", grams: 100, sort_order: 0 },
    ],
  })
  return { composed, stub }
}

export async function createVariantViaAPI(parentId: number) {
  const ctx = await apiRequest.newContext({ baseURL: API })
  const res = await ctx.post(`/api/foods/${parentId}/variant`)
  const body = await res.json()
  expect(
    res.ok(),
    `Create variant failed: ${res.status()} ${JSON.stringify(body)}`
  ).toBeTruthy()
  await ctx.dispose()
  return body as { id: number; name: string; kind: "composed" }
}

// ── Slot seeding ──────────────────────────────────────────────────────

export async function seedSlot(
  name_key: string,
  icon: string,
  sort_order: number
) {
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

/** Deletes the slot's plates from a year back to two years ahead. The API
 *  clears at most 366 days at a time, so this pages through the range. */
export async function deletePlatesUsingSlot(slotId: number) {
  const ctx = await apiRequest.newContext({ baseURL: API })
  const today = new Date().toISOString().slice(0, 10)
  for (let from = addDays(today, -360); from < addDays(today, 720); ) {
    const to = addDays(from, 359)
    const res = await ctx.delete(
      `/api/plan?from=${from}&to=${to}&slot_id=${slotId}`
    )
    expect(res.ok(), await res.text()).toBeTruthy()
    from = addDays(to, 1)
  }
  await ctx.dispose()
}

export async function cleanupSlot(id: number) {
  await deletePlatesUsingSlot(id)
  const ctx = await apiRequest.newContext({ baseURL: API })
  await ctx.delete(`/api/settings/slots/${id}`)
  await ctx.dispose()
}

/**
 * Seed a plate at `date`/`slotId` with one component. The component shape
 * mirrors the kind-aware POST body: composed foods carry `portions`, leaf
 * foods carry `amount` + `unit` (grams resolved server-side).
 */
export async function seedPlateWithComponent(
  date: string,
  slotId: number,
  component: { food_id: number } & (
    | { portions: number }
    | { amount: number; unit: string }
  )
): Promise<{ id: number }> {
  const ctx = await apiRequest.newContext({ baseURL: API })
  const plateRes = await ctx.post("/api/plates", {
    data: { date, slot_id: slotId },
  })
  const plate = (await plateRes.json()) as { id: number }
  expect(
    plateRes.ok(),
    `seed plate at ${date}/${slotId}: ${plateRes.status()} ${JSON.stringify(plate)}`
  ).toBeTruthy()
  const compRes = await ctx.post(`/api/plates/${plate.id}/components`, {
    data: component,
  })
  expect(
    compRes.ok(),
    `seed component on plate ${plate.id}: ${compRes.status()} ${JSON.stringify(await compRes.json())}`
  ).toBeTruthy()
  await ctx.dispose()
  return plate
}

export async function getSetting(key: string): Promise<string | null> {
  const ctx = await apiRequest.newContext({ baseURL: API })
  const res = await ctx.get("/api/settings")
  const body = (await res.json()) as { items: { key: string; value: string }[] }
  await ctx.dispose()
  return body.items.find((i) => i.key === key)?.value ?? null
}

export async function setSetting(key: string, value: string): Promise<void> {
  const ctx = await apiRequest.newContext({ baseURL: API })
  const res = await ctx.put(`/api/settings/${encodeURIComponent(key)}`, {
    data: { value },
  })
  const status = res.status()
  const body = res.ok() ? "" : await res.text()
  await ctx.dispose()
  if (!res.ok()) {
    throw new Error(`PUT /api/settings/${key} failed: ${status} ${body}`)
  }
}

/**
 * Intercepts GET /api/settings on the given page and injects plan.anchor="today".
 * Isolated to the page context — no server state modified, safe for parallel runs.
 */
export async function mockAnchorToday(page: Page) {
  await mockSettingValues(page, { "plan.anchor": "today" })
}

/**
 * Intercepts GET /api/settings on the given page and overrides the given
 * values. Page-scoped, so parallel specs never see them.
 */
export async function mockSettingValues(
  page: Page,
  values: Record<string, string>
) {
  await page.route("**/api/settings", async (route) => {
    if (route.request().method() !== "GET") {
      await route.continue()
      return
    }
    const response = await route.fetch()
    const body = (await response.json()) as {
      items: { key: string; value: string }[]
    }
    const items = body.items.map((item) =>
      item.key in values ? { ...item, value: values[item.key] } : item
    )
    for (const [key, value] of Object.entries(values)) {
      if (!items.some((i) => i.key === key)) items.push({ key, value })
    }
    await route.fulfill({
      status: response.status(),
      headers: { ...response.headers(), "content-type": "application/json" },
      body: JSON.stringify({ ...body, items }),
    })
  })
}

// ── Template seeding ──────────────────────────────────────────────────

export async function seedTemplate(data: {
  name: string
  components?: { food_id: number; portions: number }[]
}) {
  const ctx = await apiRequest.newContext({ baseURL: API })
  const res = await ctx.post("/api/templates", {
    data: { components: [], ...data },
  })
  const body = await res.json()
  expect(
    res.ok(),
    `Seed template "${data.name}" failed: ${res.status()} ${JSON.stringify(body)}`
  ).toBeTruthy()
  await ctx.dispose()
  return body as { id: number; name: string }
}

export async function cleanupTemplate(id: number) {
  const ctx = await apiRequest.newContext({ baseURL: API })
  await ctx.delete(`/api/templates/${id}`)
  await ctx.dispose()
}

// ── Plan board helpers ────────────────────────────────────────────────

/** `iso` plus `n` calendar days (YYYY-MM-DD). */
export function addDays(iso: string, n: number): string {
  const [y, m, d] = iso.split("-").map(Number)
  return new Date(Date.UTC(y, m - 1, d + n)).toISOString().slice(0, 10)
}

/** Tests started by this worker; rotates its windows. */
let windowsUsed = 0

/**
 * A future window for the running test, 8–50 windows after the anchored one
 * (inside the year cleanupSlot covers). Each parallel worker owns its own
 * band of ten windows (`parallelIndex` × 11, a spare week between bands),
 * and only one test runs per worker at a time, so concurrent tests never
 * share a window: one test's Clear this week or No plate overrides cannot
 * touch another's plates. Returns the start and the seven dates.
 */
export async function futureWindow(): Promise<{
  start: string
  days: string[]
}> {
  const ctx = await apiRequest.newContext({ baseURL: API })
  const res = await ctx.get("/api/plan")
  expect(res.ok()).toBeTruthy()
  const { window } = (await res.json()) as { window: { start: string } }
  await ctx.dispose()
  const band = baseTest.info().parallelIndex
  const weeksAhead = 8 + band * 11 + (windowsUsed++ % 10)
  const start = addDays(window.start, 7 * weeksAhead)
  return { start, days: Array.from({ length: 7 }, (_, i) => addDays(start, i)) }
}

/** The board cell at (date, slotId). */
export function planCell(page: Page, date: string, slotId: number): Locator {
  return page.locator(`[data-cell-key="${date}|${slotId}"]`)
}

/** Opens a board cell and returns its sheet. */
export async function openCellSheet(
  page: Page,
  date: string,
  slotId: number
): Promise<Locator> {
  await planCell(page, date, slotId).getByTestId("plan-cell").click()
  const sheet = page.getByTestId("plan-sheet")
  await expect(sheet).toBeVisible()
  return sheet
}

/** In the food picker: searches `name` on `tab` and stages the food. */
export async function stageFood(
  sheet: Locator,
  name: string,
  tab: "Recipes" | "Ingredients" = "Recipes"
) {
  await sheet.getByRole("tab", { name: tab }).click()
  await sheet.getByTestId("food-picker-search").fill(name)
  await sheet.getByTestId("food-picker-row").filter({ hasText: name }).click()
}

/** Opens the command palette with Ctrl/⌘+K, retrying until the root has
 *  bound the hotkey after the first render. */
export async function openPalette(page: Page): Promise<Locator> {
  const palette = page.getByRole("dialog", { name: "Command palette" })
  await expect(async () => {
    if (!(await palette.isVisible())) {
      await page.keyboard.press("ControlOrMeta+k")
    }
    await expect(palette).toBeVisible({ timeout: 1000 })
  }).toPass()
  return palette
}
