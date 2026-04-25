import {
  request as apiRequest,
  expect,
  test as baseTest,
  type Page,
} from "@playwright/test"

export const API = "http://localhost:8080"

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

export async function deletePlatesUsingSlot(slotId: number) {
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

export async function cleanupSlot(id: number) {
  await deletePlatesUsingSlot(id)
  const ctx = await apiRequest.newContext({ baseURL: API })
  await ctx.delete(`/api/settings/slots/${id}`)
  await ctx.dispose()
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
