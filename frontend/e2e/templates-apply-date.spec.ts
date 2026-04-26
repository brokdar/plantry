import {
  API,
  apiRequest,
  cleanupFood,
  cleanupSlot,
  cleanupTemplate,
  expect,
  seedLeafFood,
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

interface TemplateComponent {
  id: number
  day_offset: number
  food_id: number
  portions: number
}

interface Template {
  id: number
  name: string
  components: TemplateComponent[]
}

interface PlateListResponse {
  items: { id: number; date: string; slot_id: number }[]
}

async function seedTemplateWithComponents(
  name: string,
  components: { food_id: number; portions: number; day_offset?: number }[]
): Promise<Template> {
  const ctx = await apiRequest.newContext({ baseURL: API })
  const res = await ctx.post("/api/templates", { data: { name, components } })
  const body = await res.json()
  await ctx.dispose()
  return body as Template
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

async function deletePlate(id: number) {
  const ctx = await apiRequest.newContext({ baseURL: API })
  await ctx.delete(`/api/plates/${id}`)
  await ctx.dispose()
}

test.describe("Templates — apply with start_date", () => {
  test("apply template creates plates on chosen date", async ({ page }) => {
    const tag = uid()
    const food1 = await seedLeafFood({ name: `Food1 ${tag}` })
    const food2 = await seedLeafFood({ name: `Food2 ${tag}` })
    const slot = await seedSlot(`slot.tpl_apply_${tag}`, "Moon", 930)

    const startDate = dateOffset(7)
    let templateId: number | undefined
    const createdPlateIds: number[] = []

    try {
      // Seed template with 2 components, both at day_offset 0
      const tpl = await seedTemplateWithComponents(`Tpl ${tag}`, [
        { food_id: food1.id, portions: 1, day_offset: 0 },
        { food_id: food2.id, portions: 1, day_offset: 0 },
      ])
      templateId = tpl.id

      // Navigate to /templates
      await page.goto("/templates")

      // Find the template card and click it to select
      const applyBtn = page.getByTestId(`apply-template-${templateId}`)
      await expect(applyBtn).toBeVisible()
      await applyBtn.click()

      // Fill in the start date
      const dateInput = page.getByTestId("apply-start-date")
      await expect(dateInput).toBeVisible()
      await dateInput.fill(startDate)

      // Pick first slot if not already selected
      const slotSelect = page.getByTestId("apply-slot-select")
      await expect(slotSelect).toBeVisible()
      // The component auto-selects the first slot, but ensure a value is set
      const currentValue = await slotSelect.inputValue().catch(() => "")
      if (!currentValue) {
        // Open and pick the first option
        await slotSelect.click()
        await page.getByRole("option").first().click()
      }

      // Submit
      const applyResp = page.waitForResponse(
        (r) =>
          /\/api\/templates\/\d+\/apply$/.test(r.url()) &&
          r.request().method() === "POST"
      )
      await page.getByTestId("apply-template-submit").click()
      const applied = await applyResp
      expect(applied.ok()).toBe(true)

      // Verify plates were created on startDate via API
      const plates = await getPlatesForRange(startDate, startDate)
      const createdOnDate = plates.items.filter((p) => p.date === startDate)
      expect(createdOnDate.length).toBeGreaterThanOrEqual(1)
      createdPlateIds.push(...createdOnDate.map((p) => p.id))

      // Navigate to / and verify toast or plates visible (if in current window)
      await page.goto("/")
      // Navigate forward to the startDate window if needed
      // (startDate is 7 days out so may need one click of Next 7)
      const platesResp = page.waitForResponse(
        (r) => r.url().includes("/api/plates") && r.request().method() === "GET"
      )
      await page.getByRole("button", { name: /next 7/i }).click()
      await platesResp

      // Verify via API rather than UI since food names use food_id not food name
      const platesAfterNav = await getPlatesForRange(startDate, startDate)
      expect(
        platesAfterNav.items.filter((p) => p.date === startDate).length
      ).toBeGreaterThanOrEqual(1)
    } finally {
      for (const id of createdPlateIds) {
        await deletePlate(id)
      }
      // Also clean up any extra plates that may have been created by apply
      const remaining = await getPlatesForRange(startDate, startDate)
      for (const p of remaining.items) {
        await deletePlate(p.id)
      }
      if (templateId !== undefined) await cleanupTemplate(templateId)
      await cleanupFood(food1.id)
      await cleanupFood(food2.id)
      await cleanupSlot(slot.id)
    }
  })

  test("apply template — slot select pre-fills to first available slot", async ({
    page,
  }) => {
    const tag = uid()
    const food = await seedLeafFood({ name: `Food3 ${tag}` })
    const slot = await seedSlot(`slot.tpl_slot_${tag}`, "Sun", 929)

    const startDate = todayISO()
    let templateId: number | undefined
    const createdPlateIds: number[] = []

    try {
      const tpl = await seedTemplateWithComponents(`TplSlot ${tag}`, [
        { food_id: food.id, portions: 1, day_offset: 0 },
      ])
      templateId = tpl.id

      await page.goto("/templates")

      const applyBtn = page.getByTestId(`apply-template-${templateId}`)
      await expect(applyBtn).toBeVisible()
      await applyBtn.click()

      // The form should appear with start date pre-filled to today
      const dateInput = page.getByTestId("apply-start-date")
      await expect(dateInput).toBeVisible()
      const filledDate = await dateInput.inputValue()
      expect(filledDate).toBe(startDate)

      // Submit button should be enabled when a slot is pre-selected
      const submitBtn = page.getByTestId("apply-template-submit")
      await expect(submitBtn).toBeEnabled()

      const applyResp = page.waitForResponse(
        (r) =>
          /\/api\/templates\/\d+\/apply$/.test(r.url()) &&
          r.request().method() === "POST"
      )
      await submitBtn.click()
      const applied = await applyResp
      expect(applied.ok()).toBe(true)

      const plates = await getPlatesForRange(startDate, startDate)
      createdPlateIds.push(
        ...plates.items.filter((p) => p.date === startDate).map((p) => p.id)
      )
      expect(createdPlateIds.length).toBeGreaterThanOrEqual(1)
    } finally {
      for (const id of createdPlateIds) {
        await deletePlate(id)
      }
      if (templateId !== undefined) await cleanupTemplate(templateId)
      await cleanupFood(food.id)
      await cleanupSlot(slot.id)
    }
  })
})
