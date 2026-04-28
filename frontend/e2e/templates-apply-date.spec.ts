import {
  API,
  apiRequest,
  cleanupFood,
  cleanupSlot,
  cleanupTemplate,
  expect,
  mockAnchorToday,
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
): Promise<{ plates: { id: number; date: string; slot_id: number }[] }> {
  const ctx = await apiRequest.newContext({ baseURL: API })
  const res = await ctx.get(`/api/plates?from=${from}&to=${to}`)
  const body = await res.json()
  await ctx.dispose()
  return body as {
    plates: { id: number; date: string; slot_id: number }[]
  }
}

async function deletePlate(id: number) {
  const ctx = await apiRequest.newContext({ baseURL: API })
  await ctx.delete(`/api/plates/${id}`)
  await ctx.dispose()
}

test.describe("Templates — apply with start_date", () => {
  test.beforeEach(async ({ page }) => {
    await mockAnchorToday(page)
  })

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

      // Open planner and use picker sheet to access ApplyTemplateSection.
      await page.goto("/")
      const cell = page.locator(`[data-testid="cell-0-${slot.id}"]`).first()
      await expect(cell).toBeVisible()
      await cell.getByRole("button", { name: /plan meal/i }).click()

      const sheet = page.getByRole("dialog")
      await expect(sheet).toBeVisible()

      // Click the template card to show the apply form.
      const applyBtn = sheet.getByTestId(`apply-template-${templateId}`)
      await expect(applyBtn).toBeVisible()
      await applyBtn.click()

      // Fill in the start date.
      const dateInput = sheet.getByTestId("apply-start-date")
      await expect(dateInput).toBeVisible()
      await dateInput.fill(startDate)

      // Submit — slot is auto-selected to the first available slot.
      const applyResp = page.waitForResponse(
        (r) =>
          /\/api\/templates\/\d+\/apply$/.test(r.url()) &&
          r.request().method() === "POST"
      )
      await sheet.getByTestId("apply-template-submit").click()
      const applied = await applyResp
      expect(applied.ok()).toBe(true)

      // Verify plates were created on startDate via API.
      const plates = await getPlatesForRange(startDate, startDate)
      const createdOnDate = (plates.plates ?? []).filter(
        (p) => p.date === startDate
      )
      expect(createdOnDate.length).toBeGreaterThanOrEqual(1)
      createdPlateIds.push(...createdOnDate.map((p) => p.id))

      // Navigate to the startDate window and verify via API.
      await page.keyboard.press("Escape")
      await page.goto("/")
      const platesResp = page.waitForResponse(
        (r) => r.url().includes("/api/plates") && r.request().method() === "GET"
      )
      await page.getByRole("button", { name: /Next 7/i }).click()
      await platesResp

      const platesAfterNav = await getPlatesForRange(startDate, startDate)
      expect(
        (platesAfterNav.plates ?? []).filter((p) => p.date === startDate).length
      ).toBeGreaterThanOrEqual(1)
    } finally {
      for (const id of createdPlateIds) {
        await deletePlate(id)
      }
      // Clean up any extra plates that may have been created by apply.
      const remaining = await getPlatesForRange(startDate, startDate)
      for (const p of remaining.plates ?? []) {
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

      // Open planner and use picker sheet to access ApplyTemplateSection.
      await page.goto("/")
      const cell = page.locator(`[data-testid="cell-0-${slot.id}"]`).first()
      await expect(cell).toBeVisible()
      await cell.getByRole("button", { name: /plan meal/i }).click()

      const sheet = page.getByRole("dialog")
      await expect(sheet).toBeVisible()

      // Click the template card to show the form.
      const applyBtn = sheet.getByTestId(`apply-template-${templateId}`)
      await expect(applyBtn).toBeVisible()
      await applyBtn.click()

      // The form should appear with start date pre-filled to today.
      const dateInput = sheet.getByTestId("apply-start-date")
      await expect(dateInput).toBeVisible()
      const filledDate = await dateInput.inputValue()
      expect(filledDate).toBe(startDate)

      // Submit button should be enabled when a slot is pre-selected.
      const submitBtn = sheet.getByTestId("apply-template-submit")
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
        ...(plates.plates ?? [])
          .filter((p) => p.date === startDate)
          .map((p) => p.id)
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
