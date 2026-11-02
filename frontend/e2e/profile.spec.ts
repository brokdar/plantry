import {
  API,
  apiRequest,
  cleanupFood,
  cleanupSlot,
  expect,
  futureWindow,
  seedLeafFood,
  seedPlateWithComponent,
  seedSlot,
  test,
  uid,
} from "./helpers"

async function resetProfile() {
  const ctx = await apiRequest.newContext({ baseURL: API })
  await ctx.put("/api/profile", {
    data: {
      kcal_target: null,
      protein_pct: null,
      fat_pct: null,
      carbs_pct: null,
      dietary_restrictions: [],
      system_prompt: null,
      locale: "en",
    },
  })
  await ctx.dispose()
}

async function getProfile() {
  const ctx = await apiRequest.newContext({ baseURL: API })
  const res = await ctx.get("/api/profile")
  const body = await res.json()
  await ctx.dispose()
  return body as {
    kcal_target: number | null
    protein_pct: number | null
    fat_pct: number | null
    carbs_pct: number | null
    dietary_restrictions: string[]
    locale: string
  }
}

const isProfilePut = (r: { url(): string; request(): { method(): string } }) =>
  r.url().includes("/api/profile") && r.request().method() === "PUT"

test.describe("Profile settings (You & household)", () => {
  test.beforeEach(async () => {
    await resetProfile()
  })
  test.afterEach(async () => {
    await resetProfile()
  })

  test("the Cut card saves its targets at once", async ({ page }) => {
    await page.goto("/settings/household")
    const cut = page.getByRole("radio", { name: /^Cut/ })
    await expect(cut).not.toBeChecked()

    const saved = page.waitForResponse(isProfilePut)
    await page.getByText("Cut", { exact: true }).click()
    expect((await saved).status()).toBe(200)
    await expect(cut).toBeChecked()
    await expect(
      page.getByText(
        "Plantry aims for 1800 kcal a day: 35 % protein, 30 % fat, 35 % carbs."
      )
    ).toBeVisible()

    const profile = await getProfile()
    expect(profile.kcal_target).toBe(1800)
    expect(profile.protein_pct).toBe(35)
  })

  test("the Maintain card saves 2200 kcal", async ({ page }) => {
    await page.goto("/settings/household")
    const saved = page.waitForResponse(isProfilePut)
    await page.getByText("Maintain", { exact: true }).click()
    await saved
    expect((await getProfile()).kcal_target).toBe(2200)
  })

  test("custom targets save when focus leaves the fields", async ({ page }) => {
    await page.goto("/settings/household")
    await page.getByText("Custom", { exact: true }).click()
    await page.getByLabel(/calorie target/i).fill("2100")
    await page.getByLabel(/protein %/i).fill("30")
    await page.getByLabel(/fat %/i).fill("30")
    await page.getByLabel(/carbs %/i).fill("40")

    const saved = page.waitForResponse(isProfilePut)
    await page.getByRole("heading", { name: "You & household" }).click()
    expect((await saved).status()).toBe(200)
    const profile = await getProfile()
    expect(profile.kcal_target).toBe(2100)
    expect(profile.carbs_pct).toBe(40)
  })

  test("an invalid macro sum shows an error and is not saved", async ({
    page,
  }) => {
    let puts = 0
    page.on("request", (r) => {
      if (r.url().includes("/api/profile") && r.method() === "PUT") puts++
    })
    await page.goto("/settings/household")
    await page.getByText("Custom", { exact: true }).click()
    await page.getByLabel(/calorie target/i).fill("2000")
    await page.getByLabel(/protein %/i).fill("60")
    await page.getByLabel(/fat %/i).fill("30")
    await page.getByLabel(/carbs %/i).fill("30")
    await page.getByRole("heading", { name: "You & household" }).click()

    await expect(
      page.getByText("Protein, fat and carbs add up to more than 100 %.")
    ).toBeVisible()
    expect(puts).toBe(0)
  })

  test("the allergy chip editor adds and removes tags", async ({ page }) => {
    await page.goto("/settings/household")

    const added = page.waitForResponse(isProfilePut)
    await page.getByPlaceholder(/vegetarian/i).fill("peanuts")
    await page.keyboard.press("Enter")
    await added
    await expect(page.getByText("peanuts", { exact: true })).toBeVisible()
    expect((await getProfile()).dietary_restrictions).toEqual(["peanuts"])

    const removed = page.waitForResponse(isProfilePut)
    await page.getByRole("button", { name: /remove peanuts/i }).click()
    await removed
    await expect(page.getByText("peanuts", { exact: true })).toHaveCount(0)
  })
  // Z5 §17.5: a household of 3.5 buys 3.5 times the amounts, while the
  // daily kcal stay what one person eats.
  test("household_portions 3.5 scales the shopping amounts, not the daily kcal", async () => {
    const tag = uid()
    const ctx = await apiRequest.newContext({ baseURL: API })
    const before = (await (await ctx.get("/api/profile")).json()) as {
      household_portions: number
    }
    const { start, days } = await futureWindow()
    const slot = await seedSlot(`slot.hh-${tag}`, "Moon", 99985)
    const rice = await seedLeafFood({
      name: `Rice ${tag}`,
      kcal_100g: 100,
      protein_100g: 2,
      fat_100g: 1,
      carbs_100g: 20,
    })
    try {
      const put = await ctx.put("/api/profile", {
        data: { household_portions: 3.5 },
      })
      expect(put.ok(), await put.text()).toBeTruthy()

      // The plate takes the household's eaters.
      await seedPlateWithComponent(days[1], slot.id, {
        food_id: rice.id,
        amount: 100,
        unit: "g",
      })

      const list = await ctx.get(
        `/api/shopping-list?from=${start}&to=${days[6]}`
      )
      expect(list.ok(), await list.text()).toBeTruthy()
      const { sections } = (await list.json()) as {
        sections: { items: { food_id: number; total_grams: number }[] }[]
      }
      const item = sections
        .flatMap((s) => s.items)
        .find((i) => i.food_id === rice.id)
      expect(item?.total_grams).toBeCloseTo(350, 5)

      const plan = await ctx.get(`/api/plan?start=${start}`)
      const { days: planDays } = (await plan.json()) as {
        days: { date: string; macros: { kcal: number } }[]
      }
      expect(planDays.find((d) => d.date === days[1])?.macros.kcal).toBeCloseTo(
        100,
        5
      )
    } finally {
      await ctx.put("/api/profile", {
        data: { household_portions: before.household_portions },
      })
      await ctx.delete(`/api/plan?from=${days[1]}&to=${days[1]}`)
      await ctx.dispose()
      await cleanupSlot(slot.id)
      await cleanupFood(rice.id)
    }
  })
})
