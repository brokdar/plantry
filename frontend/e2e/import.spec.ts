import { test, expect } from "@playwright/test"

import {
  cleanupComponent,
  cleanupIngredient,
  seedIngredient,
  uid,
} from "./helpers"

// Chefkoch-shaped HTML pasted directly into the wizard. Avoids any live fetch
// of chefkoch.de — Playwright cannot intercept the backend's outbound request,
// but /api/import/extract accepts {html} first-class for exactly this reason.
function chefkochHTML(tag: string): string {
  return `<!DOCTYPE html><html lang="de"><head><meta charset="utf-8">
<script type="application/ld+json">
{
  "@context":"https://schema.org/",
  "@type":"Recipe",
  "name":"E2E Carbonara ${tag}",
  "description":"Test recipe",
  "image":["https://example.com/c.jpg"],
  "recipeYield":"4 Portionen",
  "prepTime":"PT15M",
  "cookTime":"PT20M",
  "totalTime":"PT35M",
  "recipeIngredient":[
    "400 g E2E Spaghetti ${tag}",
    "2 Zehen E2E Knoblauch ${tag}",
    "nach Geschmack Salz"
  ],
  "recipeInstructions":[
    {"@type":"HowToStep","text":"Wasser kochen."},
    {"@type":"HowToStep","text":"Spaghetti hinzufügen."}
  ]
}
</script></head><body></body></html>`
}

test.describe("Recipe Import", () => {
  test("imports a chefkoch-shaped recipe from pasted HTML", async ({
    page,
  }) => {
    const tag = uid()
    // Seed two ingredients with exact names matching the fixture so that
    // /api/import/lookup returns them with existing_id set.
    const spa = await seedIngredient({
      name: `E2E Spaghetti ${tag}`,
      kcal_100g: 350,
      protein_100g: 12,
      fat_100g: 2,
      carbs_100g: 72,
    })
    const kno = await seedIngredient({
      name: `E2E Knoblauch ${tag}`,
      kcal_100g: 149,
      protein_100g: 6,
    })

    let createdId: number | undefined

    try {
      await page.goto("/import")

      // Step 1 — paste HTML (avoids needing to hit the network).
      await page
        .getByRole("button", {
          name: /paste the page html|stattdessen das seiten-html/i,
        })
        .click()
      await page.getByLabel(/page html|seiten-html/i).fill(chefkochHTML(tag))

      const extractPromise = page.waitForResponse(
        (res) =>
          res.url().includes("/api/import/extract") &&
          res.request().method() === "POST"
      )
      await page
        .getByRole("button", { name: /extract recipe|rezept extrahieren/i })
        .click()
      const extractRes = await extractPromise
      expect(extractRes.status()).toBe(200)

      // Step 2 — wait until the lookups resolve to existing ids.
      // The first ingredient line uses "E2E Spaghetti <tag>" which matches our
      // seeded ingredient; we assert the "Linked" badge appears for it.
      await expect(page.getByText(`E2E Spaghetti ${tag}`).first()).toBeVisible()

      // Resolve the skip-default row (salt) manually by switching to Skip
      // (it's already skip by default because confidence=unparsed).
      // The salt row keeps resolution=skip; no action needed.

      // Wait until the "Next" button is enabled (no unresolved existing rows).
      const nextBtn = page.getByRole("button", { name: /next|weiter/i })
      await expect(nextBtn).toBeEnabled({ timeout: 10_000 })
      await nextBtn.click()

      // Step 3 — name is pre-filled, role defaults to "main".
      await expect(
        page.getByRole("heading", { name: /finalize|abschließen/i })
      ).toBeVisible()

      const componentPromise = page.waitForResponse(
        (res) =>
          res.url().match(/\/api\/components$/) !== null &&
          res.request().method() === "POST"
      )
      await page
        .getByRole("button", { name: /save component|komponente speichern/i })
        .click()
      const componentRes = await componentPromise
      expect(componentRes.status()).toBe(201)
      const body = (await componentRes.json()) as {
        id: number
        ingredients: { unit: string }[]
      }
      createdId = body.id
      // All ingredients must be canonicalized to g or ml.
      for (const ing of body.ingredients) {
        expect(["g", "ml"]).toContain(ing.unit)
      }
    } finally {
      if (createdId) await cleanupComponent(createdId)
      await cleanupIngredient(spa.id)
      await cleanupIngredient(kno.id)
    }
  })
})
