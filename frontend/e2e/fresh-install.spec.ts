import {
  API,
  apiRequest,
  cleanupFood,
  expect,
  futureWindow,
  mockSettingValues,
  seedLeafFood,
  test,
  uid,
} from "./helpers"

/**
 * Z5 §17.1: a fresh install gets from the wizard defaults to an accepted week
 * without anyone typing a nutrient. Runs in its own `fresh-install` project,
 * right after the `setup` project walked the wizard (so the install is still
 * fresh: no recipes) and before every other spec (it removes what it makes).
 *
 * The e2e backend has neither FDC nor OFF, so the ingredient catalogue those
 * would fill is stood in for by seeded leaves with nutrition; the import
 * wizard links the recipe lines to them like it links a real lookup hit.
 */

const NUTRIENT_FIELD = /kcal|calor|protein|fat|carb|fiber|sodium/i
const NUTRIENT_KEY = /(kcal|protein|fat|carbs|fiber|sodium)_100g/

const DISHES = [
  ["Lentil stew", "Lentils", "Carrots"],
  ["Chickpea curry", "Chickpeas", "Tomatoes"],
  ["Tomato pasta", "Pasta", "Tomatoes"],
  ["Veggie chili", "Beans", "Peppers"],
  ["Rice and beans", "Rice", "Beans"],
  ["Carrot soup", "Carrots", "Potatoes"],
  ["Potato gratin", "Potatoes", "Cream"],
  ["Pepper pasta", "Pasta", "Peppers"],
  ["Bean stew", "Beans", "Carrots"],
  ["Lentil bowl", "Lentils", "Rice"],
] as const

const INGREDIENTS = [
  "Lentils",
  "Carrots",
  "Chickpeas",
  "Tomatoes",
  "Pasta",
  "Beans",
  "Peppers",
  "Rice",
  "Potatoes",
  "Cream",
]

function recipeHTML(name: string, a: string, b: string, tag: string): string {
  return `<!DOCTYPE html><html lang="en"><head><meta charset="utf-8">
<script type="application/ld+json">
{
  "@context":"https://schema.org/",
  "@type":"Recipe",
  "name":"${name} ${tag}",
  "recipeYield":"4 servings",
  "recipeIngredient":["400 g ${a} ${tag}","300 g ${b} ${tag}"],
  "recipeInstructions":[{"@type":"HowToStep","text":"Cook everything."}]
}
</script></head><body></body></html>`
}

test("a fresh install goes from the wizard defaults to an accepted week without typing a nutrient", async ({
  page,
}) => {
  test.setTimeout(240_000)
  const tag = uid()
  const ctx = await apiRequest.newContext({ baseURL: API })
  const recipeIds: number[] = []
  const leafIds: number[] = []
  const { start, days } = await futureWindow()
  const posted: string[] = []
  page.on("request", (r) => {
    if (r.url().endsWith("/api/foods") && r.method() === "POST") {
      posted.push(r.postData() ?? "")
    }
  })

  try {
    // The install is fresh: the wizard defaults are stored, no recipes yet.
    const status = await ctx.get("/api/setup")
    expect(await status.json()).toMatchObject({
      completed: true,
      recipe_count: 0,
    })
    const slots = (await (await ctx.get("/api/settings/slots")).json()) as {
      items: { name_key: string }[]
    }
    expect(slots.items.map((s) => s.name_key)).toEqual([
      "slot.breakfast",
      "slot.lunch",
      "slot.dinner",
    ])

    for (const name of INGREDIENTS) {
      const leaf = await seedLeafFood({
        name: `${name} ${tag}`,
        kcal_100g: 120,
        protein_100g: 6,
        fat_100g: 2,
        carbs_100g: 18,
      })
      leafIds.push(leaf.id)
    }
    // Seeding is the catalogue's job; typing nutrients into recipes is not.
    posted.length = 0

    // Ten recipes through the import wizard, pasted as page HTML.
    for (const [name, a, b] of DISHES) {
      await page.goto("/recipes?add=url")
      await page
        .getByRole("button", { name: "Paste the page HTML instead" })
        .click()
      await page.getByLabel("Page HTML").fill(recipeHTML(name, a, b, tag))
      await page.getByRole("button", { name: "Extract recipe" }).click()
      await expect(
        page.getByRole("heading", { name: "Review recipe" })
      ).toBeVisible()
      await expect(page.getByText(`${a} ${tag}`).first()).toBeVisible()

      // No nutrient field exists anywhere in the review.
      const sheet = page.getByTestId("import-sheet")
      await expect(
        sheet.getByRole("textbox", { name: NUTRIENT_FIELD })
      ).toHaveCount(0)
      await expect(
        sheet.getByRole("spinbutton", { name: NUTRIENT_FIELD })
      ).toHaveCount(0)

      const save = page.getByRole("button", { name: "Save recipe" })
      await expect(save).toBeEnabled({ timeout: 10_000 })
      const created = page.waitForResponse(
        (r) => r.url().endsWith("/api/foods") && r.request().method() === "POST"
      )
      await save.click()
      const res = await created
      expect(res.status(), await res.text()).toBe(201)
      recipeIds.push(((await res.json()) as { id: number }).id)
      await expect(page).toHaveURL(new RegExp(`/recipes/${recipeIds.at(-1)}$`))
    }
    expect(recipeIds).toHaveLength(10)
    expect(await (await ctx.get("/api/setup")).json()).toMatchObject({
      recipe_count: 10,
    })

    // Generate on the heuristic from the wizard's defaults, then accept.
    await mockSettingValues(page, { "ai.provider": "" })
    await page.setViewportSize({ width: 1440, height: 900 })
    await page.goto(`/plan/new?start=${start}`)
    const wizard = page.getByTestId("plan-wizard")
    await expect(
      wizard.getByRole("heading", { name: "Who's home?" })
    ).toBeVisible()
    await wizard.getByRole("button", { name: "Next" }).click()
    const generated = page.waitForResponse(
      (r) =>
        r.url().includes("/api/plan/generate") &&
        r.request().method() === "POST"
    )
    await wizard
      .getByRole("button", { name: "Generate without a note" })
      .click()
    const response = await generated
    expect(response.request().postDataJSON()).toMatchObject({
      chooser: "heuristic",
    })
    await expect(wizard).toHaveCount(0, { timeout: 60_000 })
    await expect(page).toHaveURL(new RegExp(`/plan\\?start=${start}`))
    await expect(page.getByTestId("plan-summary")).toHaveText(
      /^Draft · \d+ plates · \d+ cook days?$/
    )
    await expect(page.getByTestId("plan-warnings")).toHaveText(
      "0 plates need attention"
    )

    const accepted = page.waitForResponse(
      (r) =>
        r.url().includes("/api/plan/accept") && r.request().method() === "POST"
    )
    await page.getByRole("button", { name: "Accept and shop" }).click()
    expect((await accepted).ok()).toBeTruthy()
    await expect(page).toHaveURL(/\/shop\?/)

    // The week is confirmed and shoppable.
    const plan = (await (await ctx.get(`/api/plan?start=${start}`)).json()) as {
      cells: { plate: { state: string } | null }[]
    }
    const plates = plan.cells.flatMap((c) => (c.plate ? [c.plate] : []))
    expect(plates.length).toBeGreaterThan(0)
    expect(plates.every((p) => p.state === "confirmed")).toBe(true)
    const shop = (await (
      await ctx.get(`/api/shopping-list?from=${start}&to=${days[6]}`)
    ).json()) as { count: number }
    expect(shop.count).toBeGreaterThan(0)

    // No nutrient was typed: the recipes were saved without any, and no
    // ingredient was created by hand.
    expect(posted).toHaveLength(10)
    for (const body of posted) {
      expect(body).not.toMatch(NUTRIENT_KEY)
      expect(JSON.parse(body)).toMatchObject({ kind: "composed" })
    }
  } finally {
    await page.unrouteAll({ behavior: "ignoreErrors" })
    await ctx.delete(`/api/plan?from=${start}&to=${days[6]}`)
    for (const id of recipeIds) await cleanupFood(id)
    for (const id of leafIds) await cleanupFood(id)
    await ctx.dispose()
  }
})
