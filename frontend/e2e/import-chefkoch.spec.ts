import { test, expect } from "./helpers"

import { cleanupFood, seedLeafFood, uid } from "./helpers"

// A trimmed chefkoch.de JSON-LD block (Gemüse-Reispfanne), pasted as HTML so
// nothing is fetched from the network. It keeps the shapes that used to break
// the importer: a numeric yield, glued units ("250g"), "Zehe/n", "Zwiebel(n)"
// and the quantity-less "Salz und Pfeffer". Each ingredient name carries the
// run tag so it matches exactly one seeded leaf.
function chefkochHTML(tag: string): string {
  return `<!DOCTYPE html><html lang="de"><head><meta charset="utf-8">
<script type="application/ld+json">
{
  "@context": "http://schema.org",
  "@type": "Recipe",
  "name": "Gemüse-Reispfanne ${tag}",
  "description": "Schnelle Reispfanne mit Tomaten und Knoblauch.",
  "recipeYield": 4,
  "prepTime": "P0DT0H15M",
  "cookTime": "P0DT0H25M",
  "totalTime": "P0DT0H40M",
  "recipeCategory": "Hauptspeise",
  "recipeIngredient": [
    "250g Reis ${tag}",
    "2 Zwiebel(n) ${tag}",
    "2 Zehe/n Knoblauch ${tag}",
    "1 EL Olivenöl ${tag}",
    "400g Tomaten ${tag}",
    "Salz und Pfeffer"
  ],
  "recipeInstructions": "Reis garen. Zwiebeln und Knoblauch in Olivenöl anbraten, Tomaten zugeben und mit dem Reis mischen. Mit Salz und Pfeffer abschmecken.",
  "nutrition": {
    "@type": "NutritionInformation",
    "servingSize": "1",
    "calories": "520 kcal"
  }
}
</script></head><body></body></html>`
}

// Per-100 g values seeded for each ingredient, and the grams the parser
// yields for its line (C3 defaults: count without unit = 120 g per piece,
// Zehe = 5 g, EL = 15 ml at 1 g/ml since the leaf has no ml portion):
//
//   Reis       250 g          × 350 kcal = 875.0
//   Zwiebel    2 × 120 = 240 g × 40 kcal =  96.0
//   Knoblauch  2 × 5   =  10 g × 149 kcal =  14.9
//   Olivenöl   1 × 15  =  15 g × 884 kcal = 132.6
//   Tomaten    400 g          ×  18 kcal =  72.0
//   Salz und Pfeffer: no quantity, skipped
//                                  total = 1190.5 kcal / 4 portions
const EXPECTED_KCAL_PER_PORTION = 1190.5 / 4 // 297.625

const INGREDIENTS = [
  {
    name: "Reis",
    kcal_100g: 350,
    protein_100g: 7,
    fat_100g: 0.6,
    carbs_100g: 78,
  },
  {
    name: "Zwiebel",
    kcal_100g: 40,
    protein_100g: 1.1,
    fat_100g: 0.1,
    carbs_100g: 9,
  },
  {
    name: "Knoblauch",
    kcal_100g: 149,
    protein_100g: 6.4,
    fat_100g: 0.5,
    carbs_100g: 33,
  },
  {
    name: "Olivenöl",
    kcal_100g: 884,
    protein_100g: 0,
    fat_100g: 100,
    carbs_100g: 0,
  },
  {
    name: "Tomaten",
    kcal_100g: 18,
    protein_100g: 0.9,
    fat_100g: 0.2,
    carbs_100g: 3.9,
  },
]

test.describe("Recipe import — chefkoch", () => {
  test("auto-matched chefkoch recipe saves with the expected kcal per portion", async ({
    page,
  }) => {
    const tag = `e2e-${uid()}`
    const leaves: { id: number; name: string }[] = []
    let createdId: number | undefined

    try {
      for (const ing of INGREDIENTS) {
        leaves.push(await seedLeafFood({ ...ing, name: `${ing.name} ${tag}` }))
      }
      // No lookup stub: the backend runs with PLANTRY_OFF_DISABLED and no FDC
      // key, so /api/import/lookup answers from the local catalogue alone.
      // The seeded leaf whose name equals the parsed ingredient name is the
      // exact match C1 ranks first; a line the parser mangled would match
      // nothing (or a weaker prefix hit) and fail the child-id check below.

      await page.goto("/import")
      await page.getByRole("button", { name: /paste the page html/i }).click()
      await page.getByLabel(/page html/i).fill(chefkochHTML(tag))

      const extractPromise = page.waitForResponse(
        (res) =>
          res.url().includes("/api/import/extract") &&
          res.request().method() === "POST"
      )
      await page.getByRole("button", { name: /extract recipe/i }).click()
      expect((await extractPromise).status()).toBe(200)

      await expect(
        page.getByRole("heading", { name: /review recipe/i })
      ).toBeVisible()
      await expect(page.getByLabel(/reference portions/i)).toHaveValue("4")

      // Every row auto-links; nothing is touched by hand.
      await expect(page.getByText("Linked", { exact: true })).toHaveCount(
        INGREDIENTS.length
      )
      await expect(page.getByTestId("import-nutrition")).toContainText(
        "298 kcal per portion"
      )
      await expect(page.getByTestId("import-nutrition")).toContainText(
        "Source: 520 kcal"
      )

      const save = page.getByRole("button", { name: /save recipe/i })
      await expect(save).toBeEnabled()
      const createPromise = page.waitForResponse(
        (res) =>
          /\/api\/foods$/.test(res.url()) && res.request().method() === "POST"
      )
      await save.click()
      const createRes = await createPromise
      expect(createRes.status()).toBe(201)
      const created = (await createRes.json()) as {
        id: number
        children: { child_id: number }[]
      }
      createdId = created.id
      // Each row auto-linked to its own seeded leaf.
      expect(created.children.map((c) => c.child_id).sort()).toEqual(
        leaves.map((l) => l.id).sort()
      )

      // The recipe detail shows the kcal computed by the backend.
      await expect(
        page.getByRole("heading", {
          level: 1,
          name: `Gemüse-Reispfanne ${tag}`,
        })
      ).toBeVisible()
      const kcal = page
        .getByTestId("recipe-facts")
        .locator("div", { hasText: "kcal per portion" })
        .locator("dd")
      await expect(kcal).toHaveText(/^\d+$/)
      const shown = Number(await kcal.textContent())
      expect(Math.abs(shown - EXPECTED_KCAL_PER_PORTION)).toBeLessThanOrEqual(1)
      await expect(
        page.getByTestId("nutrition-completeness-warning")
      ).toHaveCount(0)
    } finally {
      if (createdId) await cleanupFood(createdId)
      for (const leaf of leaves) await cleanupFood(leaf.id)
    }
  })
})
