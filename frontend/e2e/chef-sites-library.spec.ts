import {
  API,
  apiRequest,
  cleanupFood,
  expect,
  seedComposedFood,
  seedLeafFood,
  setSetting,
  test,
  uid,
} from "./helpers"

/**
 * E2: the recipe sites list in Settings -> Assistant (FIND-7) and the
 * monthly library review card on Recipes (LIB-1..3). chef.* is
 * process-global, so this runs in global-settings.
 */

const ctxFor = () => apiRequest.newContext({ baseURL: API })

async function storedSites(): Promise<
  { domain: string; lang: string; enabled: boolean }[]
> {
  const ctx = await ctxFor()
  const res = await ctx.get("/api/settings")
  const body = (await res.json()) as { items: { key: string; value: string }[] }
  await ctx.dispose()
  const raw = body.items.find((i) => i.key === "chef.sites")?.value ?? "[]"
  return JSON.parse(raw) as {
    domain: string
    lang: string
    enabled: boolean
  }[]
}

test.afterEach(async () => {
  const ctx = await ctxFor()
  for (const key of [
    "chef.enabled",
    "chef.sites",
    "chef.library_review_enabled",
  ]) {
    await ctx.delete(`/api/settings/${key}`)
  }
  await ctx.dispose()
})

test("a site is switched off, one is added and removed", async ({ page }) => {
  await setSetting("chef.enabled", "true")
  await page.goto("/settings/assistant")

  const sites = page.getByTestId("chef-sites")
  await expect(sites).toBeVisible()
  const toggle = page.getByLabel("Search budgetbytes.com")
  await expect(toggle).toBeChecked()
  await toggle.click()
  await expect(toggle).not.toBeChecked()
  expect(
    (await storedSites()).find((s) => s.domain === "budgetbytes.com")?.enabled
  ).toBe(false)

  // A pasted address is normalised; an address that is no site is refused.
  const domain = `${uid("site")}.example.com`
  await page.getByTestId("site-domain-input").fill("not a site")
  await page.getByTestId("site-add-button").click()
  await expect(page.getByTestId("site-domain-input")).toHaveValue("not a site")

  await page.getByTestId("site-domain-input").fill(`https://www.${domain}/x`)
  await page.getByTestId("site-lang-select").selectOption("de")
  await page.getByTestId("site-add-button").click()
  const row = page.getByTestId(`site-${domain}`)
  await expect(row).toBeVisible()
  await expect(page.getByTestId("chef-sites-de")).toContainText(domain)
  expect((await storedSites()).find((s) => s.domain === domain)).toEqual(
    expect.objectContaining({ lang: "de", enabled: true })
  )

  // A built-in site has no remove button; the added one does.
  await expect(page.getByTestId("site-remove-budgetbytes.com")).toHaveCount(0)
  await page.getByTestId(`site-remove-${domain}`).click()
  await expect(row).toHaveCount(0)
  expect((await storedSites()).some((s) => s.domain === domain)).toBe(false)
})

test("the review switch shows a card on Recipes that can be dismissed", async ({
  page,
}) => {
  await setSetting("chef.enabled", "true")
  // A recipe with no times and no steps: the review has data to ask for.
  const leaf = await seedLeafFood({ name: uid("Flour"), kcal_100g: 350 })
  const recipe = await seedComposedFood({
    children: [
      { child_id: leaf.id, amount: 100, unit: "g", grams: 100, sort_order: 0 },
    ],
    name: uid("Unfinished"),
    role: "main",
  })
  try {
    await page.goto("/recipes")
    await expect(page.getByTestId("fill-library")).toHaveCount(0)
    await expect(page.getByTestId("library-review")).toHaveCount(0)

    await page.goto("/settings/assistant")
    await page.getByLabel("Monthly library check-up").click()
    await expect(page.getByLabel("Monthly library check-up")).toBeChecked()

    await page.goto("/recipes")
    const card = page.getByTestId("library-review")
    await expect(card).toBeVisible()
    await expect(card.getByTestId("review-finding-data")).toBeVisible()

    await card.getByTestId("review-finding-dismiss-data").click()
    await expect(card.getByTestId("review-finding-data")).toHaveCount(0)

    await page.reload()
    await expect(page.getByTestId("library-review")).toHaveCount(0)
  } finally {
    await cleanupFood(recipe.id)
    await cleanupFood(leaf.id)
  }
})
