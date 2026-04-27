import {
  API,
  apiRequest,
  cleanupFood,
  cleanupSlot,
  cleanupTemplate,
  expect,
  seedComposedWithStub,
  seedSlot,
  test,
  uid,
} from "./helpers"

function todayISO(): string {
  const d = new Date()
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`
}

function dateOffset(n: number): string {
  const d = new Date()
  d.setDate(d.getDate() + n)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`
}

async function seedPlateByDate(
  date: string,
  slotId: number,
  foodId: number
): Promise<{ id: number }> {
  const ctx = await apiRequest.newContext({ baseURL: API })
  const res = await ctx.post("/api/plates", {
    data: { date, slot_id: slotId },
  })
  const plate = (await res.json()) as { id: number }
  await ctx.post(`/api/plates/${plate.id}/components`, {
    data: { food_id: foodId, portions: 1 },
  })
  await ctx.dispose()
  return plate
}

async function deletePlate(id: number) {
  const ctx = await apiRequest.newContext({ baseURL: API })
  await ctx.delete(`/api/plates/${id}`)
  await ctx.dispose()
}

interface TemplateDetail {
  id: number
  name: string
  components: { id: number; day_offset: number; food_id: number }[]
}

async function getTemplate(id: number): Promise<TemplateDetail> {
  const ctx = await apiRequest.newContext({ baseURL: API })
  const res = await ctx.get(`/api/templates/${id}`)
  const body = await res.json()
  await ctx.dispose()
  return body as TemplateDetail
}

async function getTemplates(): Promise<{
  items: { id: number; name: string }[]
}> {
  const ctx = await apiRequest.newContext({ baseURL: API })
  const res = await ctx.get("/api/templates")
  const body = await res.json()
  await ctx.dispose()
  return body as { items: { id: number; name: string }[] }
}

test.describe("Templates — save from date range", () => {
  test("save plates from today+tomorrow creates a template with day_offset 0 and 1", async ({
    page,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.tpl_save_${tag}`, "Moon", 920)
    const { composed: food0, stub: stub0 } = await seedComposedWithStub(
      { name: `Soup ${tag}`, role: "main" },
      tag
    )
    const { composed: food1, stub: stub1 } = await seedComposedWithStub(
      { name: `Stew ${tag}`, role: "main" },
      `${tag}b`
    )

    const today = todayISO()
    const tomorrow = dateOffset(1)
    const plateIds: number[] = []
    let templateId: number | undefined

    try {
      // Seed two plates: today and tomorrow
      const p0 = await seedPlateByDate(today, slot.id, food0.id)
      const p1 = await seedPlateByDate(tomorrow, slot.id, food1.id)
      plateIds.push(p0.id, p1.id)

      await page.goto("/")

      // Ensure we're on today's window
      await expect(page.getByTestId("planner-toolbar")).toBeVisible()

      // Toolbar "Save as template" saves the entire visible date range,
      // preserving each plate's day_offset relative to the window start.
      await page.getByTestId("save-range-template").click()

      // Dialog opens — fill in the template name
      const tplName = `RangeTemplate ${tag}`
      const createTplResp = page.waitForResponse(
        (r) =>
          /\/api\/templates$/.test(r.url()) && r.request().method() === "POST"
      )
      const nameInput = page.getByLabel(/template name/i)
      await expect(nameInput).toBeVisible()
      await nameInput.fill(tplName)
      await page.getByRole("button", { name: /create template/i }).click()
      const created = await createTplResp
      expect(created.status()).toBe(201)
      templateId = ((await created.json()) as { id: number }).id

      // Verify via GET /api/templates that the template exists by name
      const list = await getTemplates()
      const found = list.items.find((t) => t.name === tplName)
      expect(found).toBeDefined()
      expect(found!.id).toBe(templateId)

      // Verify via GET /api/templates/:id that both today + tomorrow produced
      // components, with day_offsets 0 and 1.
      const detail = await getTemplate(templateId!)
      expect(detail.components.length).toBeGreaterThanOrEqual(2)
      const offsets = detail.components.map((c) => c.day_offset)
      expect(offsets).toContain(0)
      expect(offsets).toContain(1)
    } finally {
      for (const id of plateIds) {
        await deletePlate(id)
      }
      if (templateId !== undefined) await cleanupTemplate(templateId)
      await cleanupFood(food0.id)
      await cleanupFood(stub0.id)
      await cleanupFood(food1.id)
      await cleanupFood(stub1.id)
      await cleanupSlot(slot.id)
    }
  })

  test("saved template appears on /templates page", async ({ page }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.tpl_page_${tag}`, "Sun", 919)
    const { composed: food, stub } = await seedComposedWithStub(
      { name: `Noodles ${tag}`, role: "main" },
      tag
    )

    const today = todayISO()
    let plateId: number | undefined
    let templateId: number | undefined

    try {
      const p = await seedPlateByDate(today, slot.id, food.id)
      plateId = p.id

      await page.goto("/")
      await expect(page.getByTestId("planner-toolbar")).toBeVisible()

      await page.getByTestId("save-range-template").click()

      const tplName = `PageTemplate ${tag}`
      const createTplResp = page.waitForResponse(
        (r) =>
          /\/api\/templates$/.test(r.url()) && r.request().method() === "POST"
      )
      await page.getByLabel(/template name/i).fill(tplName)
      await page.getByRole("button", { name: /create template/i }).click()
      const created = await createTplResp
      expect(created.status()).toBe(201)
      templateId = ((await created.json()) as { id: number }).id

      // Navigate to /templates and verify the template is listed
      await page.goto("/templates")
      await expect(page.getByText(tplName)).toBeVisible()
    } finally {
      if (plateId) await deletePlate(plateId)
      if (templateId !== undefined) await cleanupTemplate(templateId)
      await cleanupFood(food.id)
      await cleanupFood(stub.id)
      await cleanupSlot(slot.id)
    }
  })
})
