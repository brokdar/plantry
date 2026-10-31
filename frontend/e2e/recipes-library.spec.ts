import type { Page } from "@playwright/test"

import {
  addDays,
  API,
  apiRequest,
  cleanupFood,
  cleanupSlot,
  expect,
  seedComposedWithStub,
  test,
  uid,
} from "./helpers"

function localToday() {
  const d = new Date()
  const pad = (n: number) => String(n).padStart(2, "0")
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

/**
 * An inactive slot of this test's own. Its plates count for history (B7
 * derives it from every plate) but no board shows them, and no other
 * spec's cleanup (Clear week, routines, slot cleanup) can reach them.
 */
async function seedHiddenSlot(tag: string): Promise<number> {
  const ctx = await apiRequest.newContext({ baseURL: API })
  const res = await ctx.post("/api/settings/slots", {
    data: {
      name_key: `slot.lib-history-${tag}`,
      icon: "Moon",
      sort_order: 3_000_000,
      active: false,
    },
  })
  expect(res.ok(), await res.text()).toBeTruthy()
  const slot = (await res.json()) as { id: number }
  await ctx.dispose()
  return slot.id
}

/** A past plate holding `foodId` in `slotId`. */
async function seedEatenOn(
  slotId: number,
  date: string,
  foodId: number
): Promise<void> {
  const ctx = await apiRequest.newContext({ baseURL: API })
  const res = await ctx.post("/api/plates", {
    data: { date, slot_id: slotId, components: [{ food_id: foodId }] },
  })
  expect(res.ok(), await res.text()).toBeTruthy()
  await ctx.dispose()
}

/** Narrows the library to just-seeded recipes (the e2e DB accumulates). */
async function searchFor(page: Page, tag: string) {
  const res = page.waitForResponse(
    (r) => r.url().includes("/api/foods") && r.url().includes(`search=${tag}`)
  )
  await page.getByTestId("recipes-search").fill(tag)
  await res
}

test.describe("Recipe library", () => {
  test("cards show the name and meta line and open the recipe", async ({
    page,
  }) => {
    const tag = uid()
    const { composed, stub } = await seedComposedWithStub(
      {
        name: `Card Main ${tag}`,
        role: "main",
        prep_minutes: 10,
        cook_minutes: 15,
      },
      tag
    )
    try {
      await page.goto("/recipes")
      await searchFor(page, tag)

      const card = page.getByTestId(`recipe-card-${composed.id}`)
      await expect(card.getByRole("heading")).toHaveText(composed.name)
      await expect(
        page.getByTestId(`recipe-card-${composed.id}-meta`)
      ).toHaveText(/^25 min · .*never eaten$/)

      await card.click()
      await expect(page).toHaveURL(new RegExp(`/recipes/${composed.id}$`))
      await expect(
        page.getByRole("heading", { level: 1, name: composed.name })
      ).toBeVisible()
    } finally {
      await cleanupFood(composed.id)
      await cleanupFood(stub.id)
    }
  })

  test("F8-AC1: last eaten, Not eaten lately and Sort by last eaten", async ({
    page,
  }) => {
    const tag = uid()
    const today = localToday()
    const seeded = await Promise.all([
      seedComposedWithStub({ name: `Recent ${tag}`, role: "main" }, tag),
      seedComposedWithStub({ name: `Never ${tag}`, role: "main" }, tag),
      seedComposedWithStub({ name: `Long ago ${tag}`, role: "main" }, tag),
    ])
    const [recent, never, longAgo] = seeded.map((s) => s.composed)
    const slot = await seedHiddenSlot(tag)
    await seedEatenOn(slot, addDays(today, -3), recent.id)
    await seedEatenOn(slot, addDays(today, -42), longAgo.id)
    try {
      await page.goto("/recipes")
      await searchFor(page, tag)

      await expect(
        page.getByTestId(`recipe-card-${recent.id}-meta`)
      ).toContainText("eaten 3 days ago")
      await expect(
        page.getByTestId(`recipe-card-${longAgo.id}-meta`)
      ).toContainText("eaten 6 weeks ago")
      await expect(
        page.getByTestId(`recipe-card-${never.id}-meta`)
      ).toContainText("never eaten")

      await test.step("Not eaten lately hides the recipe eaten 3 days ago", async () => {
        const res = page.waitForResponse((r) =>
          r.url().includes("not_eaten_weeks=4")
        )
        await page.getByRole("button", { name: "Not eaten lately" }).click()
        await res
        await expect(page.getByTestId(`recipe-card-${recent.id}`)).toHaveCount(
          0
        )
        await expect(page.getByTestId(`recipe-card-${never.id}`)).toBeVisible()
        await expect(
          page.getByTestId(`recipe-card-${longAgo.id}`)
        ).toBeVisible()
        await expect(page).toHaveURL(/stale=true/)
      })

      await test.step("the filter survives a reload", async () => {
        await page.reload()
        await expect(
          page.getByRole("button", { name: "Not eaten lately" })
        ).toHaveAttribute("aria-pressed", "true")
        await expect(page.getByTestId("recipes-search")).toHaveValue(tag)
        await expect(page.getByTestId(`recipe-card-${recent.id}`)).toHaveCount(
          0
        )
        await page.getByRole("button", { name: "Not eaten lately" }).click()
      })

      await test.step("Sort by last eaten puts never-eaten first, then oldest", async () => {
        const res = page.waitForResponse((r) =>
          r.url().includes("sort=last_eaten")
        )
        await page.getByRole("combobox", { name: "Sort" }).click()
        await page.getByRole("option", { name: "Last eaten" }).click()
        await res
        await expect(
          page.getByTestId("recipes-grid").getByRole("heading")
        ).toHaveText([never.name, longAgo.name, recent.name])
        await expect(page).toHaveURL(/sort=last_eaten/)
      })
    } finally {
      await cleanupSlot(slot)
      for (const s of seeded) {
        await cleanupFood(s.composed.id)
        await cleanupFood(s.stub.id)
      }
    }
  })

  test("the role filter narrows the grid", async ({ page }) => {
    const tag = uid()
    const { composed: main, stub: mainStub } = await seedComposedWithStub(
      { name: `Chip Main ${tag}`, role: "main" },
      tag
    )
    const { composed: sauce, stub: sauceStub } = await seedComposedWithStub(
      { name: `Chip Sauce ${tag}`, role: "sauce" },
      tag
    )
    try {
      await page.goto("/recipes")
      await searchFor(page, tag)
      await expect(page.getByTestId(`recipe-card-${sauce.id}`)).toBeVisible()

      await page.getByRole("combobox", { name: "Role" }).click()
      await page.getByRole("option", { name: "Main" }).click()
      await expect(page.getByTestId(`recipe-card-${sauce.id}`)).toHaveCount(0)
      await expect(page.getByTestId(`recipe-card-${main.id}`)).toBeVisible()
      await expect(page).toHaveURL(/role=main/)

      // Clear filters also empties the search box (an outside change).
      await page.getByRole("button", { name: "Clear filters" }).click()
      await expect(page.getByTestId("recipes-search")).toHaveValue("")
      await expect(page).not.toHaveURL(/role=|q=/)
    } finally {
      await cleanupFood(main.id)
      await cleanupFood(sauce.id)
      await cleanupFood(mainStub.id)
      await cleanupFood(sauceStub.id)
    }
  })

  test("typing in the middle of the search keeps the caret", async ({
    page,
  }) => {
    await page.goto("/recipes?q=chicken%20soup")
    const box = page.getByTestId("recipes-search")
    await expect(box).toHaveValue("chicken soup")
    await box.click()
    await box.press("Home")
    await box.press("ArrowRight")
    await box.press("ArrowRight")
    await box.pressSequentially("XY")
    await expect(box).toHaveValue("chXYicken soup")
    await expect(page).toHaveURL(/q=chXYicken/)
  })

  test("a search without results says so", async ({ page }) => {
    await page.goto("/recipes")
    await searchFor(page, `zzz-${uid()}`)
    await expect(page.getByTestId("recipes-empty")).toContainText(
      "No recipes match"
    )
  })

  test("the Add menu opens the import sheet and Create manually", async ({
    page,
  }) => {
    await page.goto("/recipes?fav=true")
    await page.getByRole("button", { name: "Add recipe" }).click()
    await page.getByRole("menuitem", { name: "Import from a link" }).click()
    const sheet = page.getByRole("dialog", { name: "Import recipe" })
    await expect(sheet).toBeVisible()
    await expect(page).toHaveURL(/add=url/)

    await sheet.getByRole("button", { name: "Close" }).click()
    await expect(sheet).toHaveCount(0)
    await expect(page).not.toHaveURL(/add=/)
    await expect(page).toHaveURL(/fav=true/)

    await page.getByRole("button", { name: "Add recipe" }).click()
    await page.getByRole("menuitem", { name: "Create manually" }).click()
    await expect(page).toHaveURL(/\/recipes\/new$/)
  })

  test("the Ingredients tab switches to the table", async ({ page }) => {
    await page.goto("/recipes")
    const tabs = page.getByRole("navigation", { name: "Recipe sections" })
    await expect(tabs.getByRole("link", { name: "Recipes" })).toHaveAttribute(
      "aria-current",
      "page"
    )
    await tabs.getByRole("link", { name: "Ingredients" }).click()
    await expect(page).toHaveURL(/\/recipes\/ingredients$/)
    await expect(
      tabs.getByRole("link", { name: "Ingredients" })
    ).toHaveAttribute("aria-current", "page")
  })

  test("F8-AC4: the old paths render not-found", async ({ page }) => {
    for (const path of ["/components", "/ingredients", "/import"]) {
      await page.goto(path)
      await expect(page.getByTestId("not-found")).toBeVisible()
    }
  })
})
