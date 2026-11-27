import { test, expect } from "./helpers"

import { cleanupFood, seedLeafFood, uid } from "./helpers"

// Chefkoch-shaped HTML pasted directly into the wizard. Avoids any live fetch
// of chefkoch.de — Playwright cannot intercept the backend's outbound request,
// but /api/import/extract accepts {html} first-class for exactly this reason.
// The image points at port 9 on loopback, which refuses the connection: the
// download fails at once without leaving the machine, so the save warns.
function chefkochHTML(tag: string, extraLines: string[] = []): string {
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
    "2 Zehen E2E Knoblauch ${tag}"${extraLines.map((l) => `,\n    ${JSON.stringify(l)}`).join("")}
  ],
  "recipeInstructions":[
    {"@type":"HowToStep","text":"Wasser kochen."},
    {"@type":"HowToStep","text":"Spaghetti hinzufügen."}
  ]
}
</script></head><body></body></html>`
}

test.describe("Recipe Import", () => {
  test("at 375 px the sheet fills the screen and its controls are 44 px", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 375, height: 800 })
    await page.goto("/recipes?add=url")
    const sheet = page.getByTestId("import-sheet")
    await expect(sheet).toBeVisible()
    await expect.poll(async () => (await sheet.boundingBox())?.width).toBe(375)

    const targets = [
      page.getByLabel("Recipe URL"),
      page.getByRole("button", { name: "Extract recipe" }),
      page.getByRole("button", { name: "Paste the page HTML instead" }),
    ]
    for (const target of targets) {
      expect((await target.boundingBox())!.height).toBeGreaterThanOrEqual(44)
    }
  })

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
      fat_100g: 0.5,
      carbs_100g: 33,
    })

    let createdId: number | undefined

    try {
      // The real /api/import/lookup answers from the local catalogue (OFF is
      // disabled and FDC has no key in e2e): each seeded leaf is the exact
      // match for its line and gets auto-linked.
      await page.goto("/recipes?add=url")

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

      // Review — wait until the lookups resolve to existing ids.
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
      // Mass and volume are canonicalized to g or ml; the garlic count
      // keeps its unit (D19) with grams from the leaf's clove portion.
      expect(body.children.map((c) => c.unit).sort()).toEqual(["clove", "g"])
      // The imported description lands in description, not in notes.
      expect(body.description).toBe("Test recipe")
      expect(body.notes ?? null).toBeNull()
      // The unreachable image only warns; the recipe is still saved.
      expect(body.warnings).toEqual(["image_download_failed"])
      await expect(
        page.getByText(/photo could not be downloaded|foto konnte nicht/i)
      ).toBeVisible()

      // F8-AC2: after saving, the recipe detail opens and the sheet is gone.
      await expect(page).toHaveURL(new RegExp(`/recipes/${createdId}$`))
      await expect(page.getByTestId("import-sheet")).toHaveCount(0)
      await expect(
        page.getByRole("heading", { level: 1, name: `E2E Carbonara ${tag}` })
      ).toBeVisible()
    } finally {
      if (createdId) await cleanupFood(createdId)
      await cleanupFood(spa.id)
      await cleanupFood(kno.id)
    }
  })

  test("an unmatched line blocks Save until it is fixed; salt saves to taste (AC-10)", async ({
    page,
  }) => {
    const tag = uid()
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
      fat_100g: 0.5,
      carbs_100g: 33,
    })
    const mandrake = await seedLeafFood({
      name: `E2E Mandrake ${tag}`,
      kcal_100g: 60,
      protein_100g: 2,
      fat_100g: 0.3,
      carbs_100g: 12,
    })
    // No nutrition: a to-taste line needs none.
    const salt = await seedLeafFood({ name: `E2E Salz ${tag}` })
    const seeded = [spa.id, kno.id, mandrake.id, salt.id]
    const cleanup = [...seeded]

    try {
      await page.goto("/recipes?add=url")
      await page.getByRole("button", { name: /paste the page html/i }).click()
      await page
        .getByLabel(/page html/i)
        .fill(
          chefkochHTML(tag, [
            "nach Geschmack Salz",
            `50 g E2E Zauberwurz ${tag}`,
            `30 g E2E Alraune ${tag}`,
          ])
        )
      await page.getByRole("button", { name: /extract recipe/i }).click()
      await expect(
        page.getByRole("heading", { name: /review recipe/i })
      ).toBeVisible()

      const save = page.getByRole("button", { name: /save recipe/i })
      await expect(page.getByText(/ingredients need a choice/)).toBeVisible({
        timeout: 10_000,
      })
      await expect(save).toBeDisabled()
      await expect(page.getByText(/^Skip$/)).toHaveCount(0)
      await expect(page.getByText("to taste").first()).toBeVisible()

      // Salt needs no amount; it links to a catalogue salt when the lane's
      // data has one, else to the seeded leaf found under another name.
      const saltSearch = page.getByLabel("Search name for Salz")
      if (await saltSearch.isVisible()) {
        await saltSearch.fill(`E2E Salz ${tag}`)
        await saltSearch.press("Enter")
        await page
          .getByRole("button", { name: new RegExp(`E2E Salz ${tag}`) })
          .click()
      }
      await expect(page.getByText("2 ingredients need a choice")).toBeVisible()

      // Search again with another name and pick its catalogue hit.
      const search = page.getByLabel(`Search name for E2E Zauberwurz ${tag}`)
      await search.fill(`E2E Mandrake ${tag}`)
      await search.press("Enter")
      await page
        .getByRole("button", { name: new RegExp(`E2E Mandrake ${tag}`) })
        .click()
      await expect(page.getByText("1 ingredient needs a choice")).toBeVisible()
      await expect(save).toBeDisabled()

      // The unknown food: a new leaf only with nutrition per 100 g.
      await page.getByRole("button", { name: /create with nutrition/i }).click()
      await page.getByLabel("kcal", { exact: true }).fill("40")
      await page.getByLabel("Protein (g)").fill("1")
      await page.getByLabel("Fat (g)").fill("0.2")
      await page.getByLabel("Carbs (g)").fill("9")
      await page.getByRole("button", { name: "Create ingredient" }).click()
      await expect(save).toBeEnabled({ timeout: 10_000 })

      const created = page.waitForResponse(
        (res) =>
          res.url().match(/\/api\/foods$/) !== null &&
          res.request().method() === "POST" &&
          res.request().postDataJSON()?.kind === "composed"
      )
      await save.click()
      const res = await created
      expect(res.status()).toBe(201)
      const body = (await res.json()) as {
        id: number
        children: { child_id: number; to_taste: boolean }[]
      }
      cleanup.unshift(body.id)
      const toTaste = body.children.filter((c) => c.to_taste)
      const added = body.children.filter(
        (c) => !c.to_taste && !seeded.includes(c.child_id)
      )
      cleanup.push(...added.map((c) => c.child_id))
      expect(body.children).toHaveLength(5)
      expect(toTaste).toHaveLength(1)
      expect(added).toHaveLength(1)

      await expect(page).toHaveURL(new RegExp(`/recipes/${body.id}$`))
      await expect(page.getByText("to taste")).toBeVisible()
    } finally {
      for (const id of cleanup) await cleanupFood(id)
    }
  })
})
