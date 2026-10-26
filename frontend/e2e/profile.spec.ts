import { apiRequest, expect, test } from "./helpers"

const API = "http://localhost:8080"

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
})
