import { request as apiRequest, expect } from "@playwright/test"

export const API = "http://localhost:8080"

export function uid() {
  return crypto.randomUUID().slice(0, 8)
}

export async function seedIngredient(data: {
  name: string
  kcal_100g?: number
  protein_100g?: number
  fat_100g?: number
  carbs_100g?: number
}) {
  const ctx = await apiRequest.newContext({ baseURL: API })
  const res = await ctx.post("/api/ingredients", { data })
  const body = await res.json()
  expect(
    res.ok(),
    `Seed ingredient "${data.name}" failed: ${res.status()} ${JSON.stringify(body)}`
  ).toBeTruthy()
  await ctx.dispose()
  return body as { id: number; name: string }
}

export async function seedComponent(data: {
  name: string
  role: string
  reference_portions?: number
  ingredients?: {
    ingredient_id: number
    amount: number
    unit: string
    grams: number
    sort_order: number
  }[]
  instructions?: { step_number: number; text: string }[]
  tags?: string[]
}) {
  const ctx = await apiRequest.newContext({ baseURL: API })
  const res = await ctx.post("/api/components", {
    data: { reference_portions: 1, ...data },
  })
  const body = await res.json()
  expect(
    res.ok(),
    `Seed component "${data.name}" failed: ${res.status()} ${JSON.stringify(body)}`
  ).toBeTruthy()
  await ctx.dispose()
  return body as { id: number; name: string }
}

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

export async function cleanupComponent(id: number) {
  const ctx = await apiRequest.newContext({ baseURL: API })
  await ctx.delete(`/api/components/${id}`)
  await ctx.dispose()
}

export async function cleanupIngredient(id: number) {
  const ctx = await apiRequest.newContext({ baseURL: API })
  await ctx.delete(`/api/ingredients/${id}`)
  await ctx.dispose()
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

export async function cleanupTemplate(id: number) {
  const ctx = await apiRequest.newContext({ baseURL: API })
  await ctx.delete(`/api/templates/${id}`)
  await ctx.dispose()
}

export async function createVariantViaAPI(parentId: number) {
  const ctx = await apiRequest.newContext({ baseURL: API })
  const res = await ctx.post(`/api/components/${parentId}/variant`)
  const body = await res.json()
  expect(
    res.ok(),
    `Create variant failed: ${res.status()} ${JSON.stringify(body)}`
  ).toBeTruthy()
  await ctx.dispose()
  return body as { id: number; name: string }
}
