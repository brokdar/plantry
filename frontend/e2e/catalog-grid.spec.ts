import { cleanupComponent, expect, seedComponent, test, uid } from "./helpers"

test.describe("Recipe Catalog (card grid)", () => {
  test("renders cards with title, role, and portions", async ({ page }) => {
    const tag = uid()
    const main = await seedComponent({
      name: `Grid Main ${tag}`,
      role: "main",
      reference_portions: 2,
    })
    const sauce = await seedComponent({
      name: `Grid Sauce ${tag}`,
      role: "sauce",
    })

    try {
      await page.goto("/components")

      // Narrow to just-seeded items (avoids pagination edge cases in the
      // long-lived e2e database).
      await page.getByTestId("catalog-search").fill(tag)
      await page.waitForResponse(
        (r) =>
          r.url().includes("/api/components") &&
          r.url().includes(`search=${tag}`)
      )

      const mainCard = page.getByTestId(`component-card-${main.id}`)
      const sauceCard = page.getByTestId(`component-card-${sauce.id}`)
      await expect(mainCard).toBeVisible()
      await expect(sauceCard).toBeVisible()

      await expect(mainCard.getByText(main.name)).toBeVisible()
    } finally {
      await cleanupComponent(main.id)
      await cleanupComponent(sauce.id)
    }
  })

  test("role filter chip single-select narrows grid", async ({ page }) => {
    const tag = uid()
    const main = await seedComponent({ name: `Chip Main ${tag}`, role: "main" })
    const sauce = await seedComponent({
      name: `Chip Sauce ${tag}`,
      role: "sauce",
    })

    try {
      await page.goto("/components")
      await page.getByTestId("catalog-search").fill(tag)
      await page.waitForResponse(
        (r) =>
          r.url().includes("/api/components") &&
          r.url().includes(`search=${tag}`)
      )
      await expect(page.getByTestId(`component-card-${main.id}`)).toBeVisible()
      await expect(page.getByTestId(`component-card-${sauce.id}`)).toBeVisible()

      await page.getByTestId("component-filter-role-main").click()

      // After selection, the sauce card disappears; main stays.
      await expect(page.getByTestId(`component-card-${sauce.id}`)).toHaveCount(
        0
      )
      await expect(page.getByTestId(`component-card-${main.id}`)).toBeVisible()

      // Deselect — both visible again (within the tag-scoped search).
      await page.getByTestId("component-filter-role-main").click()
      await expect(page.getByTestId(`component-card-${main.id}`)).toBeVisible()
      await expect(page.getByTestId(`component-card-${sauce.id}`)).toBeVisible()
    } finally {
      await cleanupComponent(main.id)
      await cleanupComponent(sauce.id)
    }
  })

  test("empty-create tile navigates to /components/new", async ({ page }) => {
    await page.goto("/components")

    // Narrow to an impossible search so the empty state renders exactly one
    // create-tile regardless of how many components exist in the shared DB.
    const gibberish = `zzz-${uid()}`
    await page.getByTestId("catalog-search").fill(gibberish)
    await page.waitForResponse(
      (r) =>
        r.url().includes("/api/components") &&
        r.url().includes(`search=${gibberish}`)
    )

    await page.getByTestId("component-create-tile").click()
    await expect(page).toHaveURL(/\/components\/new$/)
  })

  test("card-menu delete confirms, removes card, and shows success", async ({
    page,
  }) => {
    const tag = uid()
    const keep = await seedComponent({
      name: `Grid Keep ${tag}`,
      role: "main",
    })
    const toDelete = await seedComponent({
      name: `Grid Delete ${tag}`,
      role: "side_veg",
    })

    try {
      await page.goto("/components")
      await page.getByTestId("catalog-search").fill(tag)
      await page.waitForResponse(
        (r) =>
          r.url().includes("/api/components") &&
          r.url().includes(`search=${tag}`)
      )
      await expect(
        page.getByTestId(`component-card-${toDelete.id}`)
      ).toBeVisible()

      await page.getByTestId(`component-card-${toDelete.id}-menu`).click()
      await page.getByTestId(`component-card-${toDelete.id}-delete`).click()

      const resp = page.waitForResponse(
        (r) =>
          r.url().includes(`/api/components/${toDelete.id}`) &&
          r.request().method() === "DELETE"
      )
      await page
        .getByRole("dialog")
        .getByRole("button", { name: "Delete", exact: true })
        .click()
      await resp

      await expect(
        page.getByTestId(`component-card-${toDelete.id}`)
      ).toHaveCount(0)
      await expect(page.getByTestId(`component-card-${keep.id}`)).toBeVisible()
    } finally {
      await cleanupComponent(keep.id)
    }
  })
})
