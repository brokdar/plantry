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

    await page.getByTestId("component-create-tile").getByRole("link").click()
    await expect(page).toHaveURL(/\/components\/new$/)
  })

  test("layout toggle switches grid ↔ list and persists across reload", async ({
    page,
  }) => {
    const tag = uid()
    const main = await seedComponent({
      name: `Toggle Main ${tag}`,
      role: "main",
    })

    try {
      await page.goto("/components")

      // Scope to seeded item so list/grid count is deterministic.
      const searchResp = page.waitForResponse(
        (r) =>
          r.url().includes("/api/components") && r.url().includes(`search=`)
      )
      await page.getByTestId("catalog-search").fill(tag)
      await searchResp

      const gridBtn = page.getByRole("button", { name: "Grid view" })
      const listBtn = page.getByRole("button", { name: "List view" })
      const card = page.getByTestId(`component-card-${main.id}`)

      await test.step("grid is the default selection", async () => {
        await expect(gridBtn).toHaveAttribute("aria-pressed", "true")
        await expect(listBtn).toHaveAttribute("aria-pressed", "false")
        await expect(card).toBeVisible()
      })

      await test.step("clicking list flips selection without losing the card", async () => {
        await listBtn.click()
        await expect(listBtn).toHaveAttribute("aria-pressed", "true")
        await expect(gridBtn).toHaveAttribute("aria-pressed", "false")
        await expect(card).toBeVisible()
      })

      await test.step("selection persists across reload", async () => {
        await page.reload()
        await expect(
          page.getByRole("button", { name: "List view" })
        ).toHaveAttribute("aria-pressed", "true")
      })

      // Reset to grid so subsequent tests start from default.
      await page.getByRole("button", { name: "Grid view" }).click()
    } finally {
      await cleanupComponent(main.id)
    }
  })

  test("tag filter chip appears from items and narrows the grid", async ({
    page,
  }) => {
    const tag = uid()
    const spicyTag = `spicy-${tag}`
    const veganTag = `vegan-${tag}`
    const spicy = await seedComponent({
      name: `Tagged Spicy ${tag}`,
      role: "main",
      tags: [spicyTag],
    })
    const vegan = await seedComponent({
      name: `Tagged Vegan ${tag}`,
      role: "main",
      tags: [veganTag],
    })

    try {
      await page.goto("/components")

      const searchResp = page.waitForResponse((r) =>
        r.url().includes(`search=`)
      )
      await page.getByTestId("catalog-search").fill(tag)
      await searchResp

      const spicyCard = page.getByTestId(`component-card-${spicy.id}`)
      const veganCard = page.getByTestId(`component-card-${vegan.id}`)
      await expect(spicyCard).toBeVisible()
      await expect(veganCard).toBeVisible()

      await test.step("clicking the spicy chip filters the grid", async () => {
        const tagResp = page.waitForResponse(
          (r) => r.url().includes("/api/components") && r.url().includes("tag=")
        )
        await page.getByTestId(`component-filter-tag-${spicyTag}`).click()
        await tagResp
        await expect(spicyCard).toBeVisible()
        await expect(veganCard).toHaveCount(0)
      })

      await test.step("clicking the active chip again removes the filter", async () => {
        // Deselecting may hit TanStack Query's cache (the unfiltered query
        // ran on first paint), so we assert UI state directly rather than
        // waiting for a network round-trip that may not fire.
        await page.getByTestId(`component-filter-tag-${spicyTag}`).click()
        await expect(spicyCard).toBeVisible()
        await expect(veganCard).toBeVisible()
      })
    } finally {
      await cleanupComponent(spicy.id)
      await cleanupComponent(vegan.id)
    }
  })

  test("clicking a card body navigates to the editor", async ({ page }) => {
    const tag = uid()
    const main = await seedComponent({
      name: `Click Body ${tag}`,
      role: "main",
    })

    try {
      await page.goto("/components")
      const searchResp = page.waitForResponse((r) =>
        r.url().includes(`search=`)
      )
      await page.getByTestId("catalog-search").fill(tag)
      await searchResp

      // The card's primary affordance is an absolute <a aria-label={name}>
      // covering the body; click it the way an accessibility tree would.
      await page
        .getByTestId(`component-card-${main.id}`)
        .getByRole("link", { name: `Click Body ${tag}`, exact: true })
        .click()

      await expect(page).toHaveURL(new RegExp(`/components/${main.id}/edit$`))
      await expect(page.getByLabel(/^name/i)).toHaveValue(`Click Body ${tag}`)
    } finally {
      await cleanupComponent(main.id)
    }
  })

  test("secondary actions menu links to import and templates", async ({
    page,
  }) => {
    await test.step("Import from URL navigates to /import", async () => {
      await page.goto("/components")
      await page.getByTestId("catalog-secondary-actions").click()
      await page.getByRole("menuitem", { name: /import from url/i }).click()
      await expect(page).toHaveURL(/\/import$/)
    })

    await test.step("Browse Templates navigates to /templates", async () => {
      await page.goto("/components")
      await page.getByTestId("catalog-secondary-actions").click()
      await page.getByRole("menuitem", { name: /browse templates/i }).click()
      await expect(page).toHaveURL(/\/templates$/)
    })
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
