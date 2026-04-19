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
    locale: string
  }
}

test.describe("Profile settings", () => {
  test.afterEach(async () => {
    await resetProfile()
  })

  test("Cut preset populates fields and saves correctly", async ({ page }) => {
    await page.goto("/settings?tab=nutrition")

    // Profile card should be visible
    await expect(page.getByText(/profile & targets/i).first()).toBeVisible()

    // Click Cut preset
    await page.getByRole("button", { name: /^cut$/i }).click()

    // Assert fields are populated
    await expect(page.getByLabel(/calorie target/i)).toHaveValue("1800")
    await expect(page.getByLabel(/protein %/i)).toHaveValue("35")
    await expect(page.getByLabel(/fat %/i)).toHaveValue("30")
    await expect(page.getByLabel(/carbs %/i)).toHaveValue("35")

    // Save
    const saveResp = page.waitForResponse(
      (r) => r.url().includes("/api/profile") && r.request().method() === "PUT"
    )
    await page.getByRole("button", { name: /save profile/i }).click()
    const response = await saveResp
    expect(response.status()).toBe(200)

    // Verify via API
    const profile = await getProfile()
    expect(profile.kcal_target).toBe(1800)
    expect(profile.protein_pct).toBe(35)
  })

  test("Maintain preset sets correct values", async ({ page }) => {
    await page.goto("/settings?tab=nutrition")
    await expect(page.getByText(/profile & targets/i).first()).toBeVisible()

    await page.getByRole("button", { name: /^maintain$/i }).click()

    await expect(page.getByLabel(/calorie target/i)).toHaveValue("2200")
  })

  test("dietary restriction chip editor adds and removes tags", async ({
    page,
  }) => {
    await page.goto("/settings?tab=nutrition")
    await expect(page.getByText(/profile & targets/i).first()).toBeVisible()

    // Add a restriction
    await page.getByPlaceholder(/vegetarian/i).fill("vegan")
    await page.keyboard.press("Enter")
    await expect(page.getByText("vegan")).toBeVisible()

    // Remove it
    await page.getByRole("button", { name: /remove vegan/i }).click()
    await expect(page.getByText("vegan")).toHaveCount(0)
  })

  test("invalid macro sum shows error from server", async ({ page }) => {
    await page.goto("/settings?tab=nutrition")
    await expect(page.getByText(/profile & targets/i).first()).toBeVisible()

    await page.getByLabel(/protein %/i).fill("60")
    await page.getByLabel(/fat %/i).fill("30")
    await page.getByLabel(/carbs %/i).fill("30")

    const saveResp = page.waitForResponse(
      (r) => r.url().includes("/api/profile") && r.request().method() === "PUT"
    )
    await page.getByRole("button", { name: /save profile/i }).click()
    const response = await saveResp
    expect(response.status()).toBe(400)
  })
})
