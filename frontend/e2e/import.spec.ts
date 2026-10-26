import { test, expect } from "./helpers"

import { cleanupFood, seedLeafFood, uid } from "./helpers"

// Chefkoch-shaped HTML pasted directly into the wizard. Avoids any live fetch
// of chefkoch.de — Playwright cannot intercept the backend's outbound request,
// but /api/import/extract accepts {html} first-class for exactly this reason.
// The image points at port 9 on loopback, which refuses the connection: the
// download fails at once without leaving the machine, so the save warns.
function chefkochHTML(tag: string): string {
  return `<!DOCTYPE html><html lang="de"><head><meta charset="utf-8">
<script type="application/ld+json">
{
  "@context":"https://schema.org/",
  "@type":"Recipe",
  "name":"E2E Carbonara ${tag}",
  "description":"Test recipe",
  "image":["http://127.0.0.1:9/c.jpg"],
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
    const spa = await seedLeafFood({
      name: `E2E Spaghetti ${tag}`,
      kcal_100g: 350,
      protein_100g: 12,
      fat_100g: 2,
      carbs_100g: 72,
    })
    const kno = await seedLeafFood({
      name: `E2E Knoblauch ${tag}`,
      kcal_100g: 149,
      protein_100g: 6,
    })

    let createdId: number | undefined

    try {
      // The backend's /api/import/lookup goes through the FDC provider, which
      // is inactive in e2e (no API key). Stub it to return the seeded local
      // ingredients so the auto-link step can select them by existing_id.
      await page.route("**/api/import/lookup*", async (route) => {
        const url = new URL(route.request().url())
        const query = url.searchParams.get("query") ?? ""
        const match = query.includes("Spaghetti")
          ? { id: spa.id, name: spa.name }
          : query.includes("Knoblauch")
            ? { id: kno.id, name: kno.name }
            : null
        const results = match
          ? [{ name: match.name, source: "fdc", existing_id: match.id }]
          : []
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            results,
            recommended_index: match ? 0 : -1,
          }),
        })
      })

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

      // Review — wait until the lookups resolve to existing ids. The salt row
      // is quantity-less, defaults to skip and does not block saving.
      await expect(
        page.getByRole("heading", { name: /review recipe|rezept prüfen/i })
      ).toBeVisible()
      await expect(page.getByText(`E2E Spaghetti ${tag}`).first()).toBeVisible()

      const saveBtn = page.getByRole("button", {
        name: /save recipe|rezept speichern/i,
      })
      await expect(saveBtn).toBeEnabled({ timeout: 10_000 })

      const componentPromise = page.waitForResponse(
        (res) =>
          res.url().match(/\/api\/foods$/) !== null &&
          res.request().method() === "POST"
      )
      await saveBtn.click()
      const componentRes = await componentPromise
      expect(componentRes.status()).toBe(201)
      const body = (await componentRes.json()) as {
        id: number
        children: { unit: string }[]
        description?: string | null
        notes?: string | null
        warnings?: string[]
      }
      createdId = body.id
      // All children must be canonicalized to g or ml.
      for (const child of body.children) {
        expect(["g", "ml"]).toContain(child.unit)
      }
      // The imported description lands in description, not in notes.
      expect(body.description).toBe("Test recipe")
      expect(body.notes ?? null).toBeNull()
      // The unreachable image only warns; the recipe is still saved.
      expect(body.warnings).toEqual(["image_download_failed"])
      await expect(
        page.getByText(/photo could not be downloaded|foto konnte nicht/i)
      ).toBeVisible()

      // After saving, the recipe detail opens.
      await expect(
        page.getByRole("heading", { level: 1, name: `E2E Carbonara ${tag}` })
      ).toBeVisible()
    } finally {
      if (createdId) await cleanupFood(createdId)
      await cleanupFood(spa.id)
      await cleanupFood(kno.id)
    }
  })
})
