import {
  API,
  apiRequest,
  expect,
  futureWindow,
  setSetting,
  test,
} from "./helpers"

/**
 * D3: with the Chef on, the plan page's theme picker sets a week theme
 * (THEME-2), says plainly that the library has none and the assistant
 * looks for two (no site is enabled, so no find reaches the web), removes
 * it, and the window after offers it again with one tap (THEME-5).
 * chef.* is process-global, so this runs in global-settings.
 */

const ctxFor = () => apiRequest.newContext({ baseURL: API })

function addDays(iso: string, n: number): string {
  const d = new Date(`${iso}T00:00:00Z`)
  d.setUTCDate(d.getUTCDate() + n)
  return d.toISOString().slice(0, 10)
}

let windows: string[] = []

test.afterEach(async () => {
  const ctx = await ctxFor()
  for (const start of windows) {
    const res = await ctx.get(`/api/windows/${start}/themes`)
    const body = (await res.json()) as { themes: { id: number }[] }
    for (const th of body.themes) {
      await ctx.delete(`/api/windows/${start}/themes/${th.id}`)
    }
  }
  for (const key of ["chef.enabled", "chef.sites"]) {
    await ctx.delete(`/api/settings/${key}`)
  }
  await ctx.dispose()
  windows = []
})

test("the picker sets a theme, says what it finds, and next week offers it again", async ({
  page,
}) => {
  await setSetting("chef.enabled", "true")
  await setSetting("chef.sites", "[]")
  const { start: base } = await futureWindow()
  // A window of its own, far from the ones other specs plan.
  const start = addDays(base, 7 * (60 + Math.floor(Math.random() * 40)))
  const next = addDays(start, 7)
  windows = [start, next]

  await page.goto(`/plan?start=${start}`)
  const bar = page.getByTestId("theme-bar")
  await bar.getByRole("button", { name: "Add theme" }).click()
  await page.getByRole("radio", { name: "Cuisine" }).click()
  await page.getByRole("textbox", { name: "Theme" }).fill("Zanzibari")
  await page.getByRole("button", { name: "Add", exact: true }).click()
  await expect(
    page.getByText("No Zanzibari recipes yet. I'll find 2.")
  ).toBeVisible()
  await expect(
    bar.getByRole("button", { name: "Remove theme Zanzibari" })
  ).toBeVisible()

  await page.goto(`/plan?start=${next}`)
  await expect(page.getByText("Last week: Zanzibari.")).toBeVisible()
  await page.getByRole("button", { name: "Again?" }).click()
  await expect(
    page
      .getByTestId("theme-bar")
      .getByRole("button", { name: "Remove theme Zanzibari" })
  ).toBeVisible()

  await page
    .getByTestId("theme-bar")
    .getByRole("button", { name: "Remove theme Zanzibari" })
    .click()
  await expect(page.getByText("Last week: Zanzibari.")).toBeVisible()
})
