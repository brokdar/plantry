import type { APIRequestContext, Page } from "@playwright/test"

import {
  clearPendingSuggestions,
  localDay,
  seedExplorer,
  SITE,
  useDiscoveryScript,
} from "./discovery-fixture"
import { API, expect, mockSettingValues, setSetting, test } from "./helpers"

// Phase 5 F4: the acceptance walk of recipe discovery (AC-1, AC-2, AC-3)
// against the real backend with the fake research client and the fixture
// recipe site. It writes global settings and the fake cursor, so it runs in
// `ai-serial`. The tests share state and run in order: the daily idea first,
// then the Discover sheet, then accepting the daily Fesenjan last (a saved
// Fesenjan would make Sumac "known" for the ranking test).
test.describe.configure({ mode: "serial" })

const DAY1 = localDay(0)
const DAY2 = localDay(1)

let explorer: Awaited<ReturnType<typeof seedExplorer>>
let script: Awaited<ReturnType<typeof useDiscoveryScript>>
const extraScripts: { remove: () => Promise<void> }[] = []

test.beforeAll(async () => {
  await clearPendingSuggestions()
  explorer = await seedExplorer()
  await setSetting("discovery.cuisines", JSON.stringify(["persian"]))
  await setSetting("discovery.daily_enabled", "true")
  // fake-e2e has no built-in price: 1 USD per million tokens, 0.50 a search.
  await setSetting(
    "ai.price_overrides",
    JSON.stringify({ "fake-e2e": { in: 1, out: 1, per_search: 0.5 } })
  )
  script = await useDiscoveryScript()
})

test.afterAll(async () => {
  await clearPendingSuggestions()
  await script.remove()
  for (const s of extraScripts) await s.remove()
  for (const key of [
    "discovery.cuisines",
    "discovery.daily_enabled",
    "ai.price_overrides",
  ]) {
    await fetch(`${API}/api/settings/${key}`, { method: "DELETE" })
  }
  await explorer.cleanup()
})

interface Run {
  id: number
  status: string
  prompt_version: string | null
  web_search_requests: number
  cost_estimate_usd: number | null
  outcome: string | null
  scope: { mode: string; cuisine?: string; focus?: string }
  notes: string | null
}

interface SuggestionRow {
  id: number
  status: string
  reason: string | null
  source_url: string
  name: string
  food_id: number | null
  grounded: boolean
  run_id: number | null
  image_url: string | null
  per_portion: { kcal: number; protein: number; fat: number; carbs: number }
}

async function discoveryRuns(request: APIRequestContext): Promise<Run[]> {
  const res = await request.get(`${API}/api/ai/runs?kind=discovery&limit=50`)
  const { items } = (await res.json()) as { items: { id: number }[] }
  const runs: Run[] = []
  for (const { id } of items) {
    runs.push(await (await request.get(`${API}/api/ai/runs/${id}`)).json())
  }
  return runs.sort((a, b) => b.id - a.id)
}

async function suggestions(
  request: APIRequestContext,
  status: string
): Promise<SuggestionRow[]> {
  const res = await request.get(
    `${API}/api/discovery/suggestions?status=${status}`
  )
  return ((await res.json()) as { items: SuggestionRow[] }).items
}

/** Pins the page's clock to noon of `day`. */
async function pinDay(page: Page, day: string) {
  await page.clock.setFixedTime(new Date(`${day}T12:00:00`))
}

/** Opens the Discover sheet from the library's Add menu. */
async function openDiscover(page: Page) {
  await page.goto("/recipes")
  await page.getByTestId("add-recipe").click()
  await page.getByRole("menuitem", { name: "Discover recipes" }).click()
  const sheet = page.getByTestId("discover-sheet")
  await expect(sheet).toBeVisible()
  return sheet
}

test("AC-1.1-1.3: the first visit of the day opens the daily idea, and the run is logged", async ({
  page,
  request,
}) => {
  await page.unroute("**/api/discovery/daily*")
  await pinDay(page, DAY1)
  await page.goto("/today")

  const sheet = page.getByTestId("daily-suggestion-sheet")
  await expect(sheet).toBeVisible({ timeout: 15_000 })
  const card = sheet.getByTestId("suggestion-card")
  await expect(card.getByRole("heading", { name: "Fesenjan" })).toBeVisible()

  // The downloaded og:image, 16:9, served from the suggestion image path.
  const photo = card.getByRole("img", { name: "Photo of Fesenjan" })
  await expect(photo).toHaveAttribute("src", /\/images\/suggestions\/\d+\.jpg$/)
  await expect
    .poll(() => photo.evaluate((el) => (el as HTMLImageElement).naturalWidth))
    .toBeGreaterThan(0)
  const box = (await photo.boundingBox())!
  expect(box.width / box.height).toBeCloseTo(16 / 9, 1)

  await expect(card).toContainText("60 min · 4 portions · Persian")
  await expect(card).toContainText(/\d+ kcal · \d+ g protein per portion/)
  await expect(card).toContainText(
    "Uses 6 ingredients you already know. New cuisine for you: Persian."
  )

  // Sumac and Pomegranate molasses are new; the other six carry no label.
  const items = card.getByRole("listitem")
  await expect(items).toHaveCount(8)
  await expect(card.getByTestId("suggestion-new")).toHaveCount(2)
  for (const name of ["Sumac", "Pomegranate molasses"]) {
    await expect(
      items.filter({ hasText: name }).getByTestId("suggestion-new")
    ).toHaveText("new")
  }
  await expect(
    items.filter({ hasText: "Chicken thighs" }).getByTestId("suggestion-new")
  ).toHaveCount(0)
  await expect(card.getByText(/“new” marks ingredients/)).toBeVisible()
  await expect(
    card.getByRole("link", { name: /View the original recipe/ })
  ).toHaveAttribute("href", `${SITE}/fesenjan.html?r=${script.run}`)
  await expect(card.getByTestId("suggestion-grounded")).toHaveText(
    "Search-grounded"
  )

  // AC-1.3: one discovery run with both searches; the unreachable and the
  // missing page made no suggestion and are counted as discarded.
  const runs = await discoveryRuns(request)
  expect(runs).toHaveLength(1)
  expect(runs[0]).toMatchObject({
    status: "ok",
    prompt_version: "d1",
    web_search_requests: 2,
    scope: { mode: "daily" },
  })
  expect(runs[0].notes).toMatch(/^2 of 4 verified; discarded: /)
  // 2 x (1,200 + 300) tokens at 1 USD per million, plus 2 searches at 0.50.
  expect(runs[0].cost_estimate_usd).toBeCloseTo(1.003, 3)
  const pending = await suggestions(request, "pending")
  expect(pending.map((s) => s.name).sort()).toEqual([
    "Buttered onion spaghetti",
    "Fesenjan",
  ])

  await page.goto(`/settings/assistant/insights?run=${runs[0].id}`)
  const detail = page.getByTestId("run-detail-sheet")
  await expect(detail.getByTestId("run-detail-searches")).toContainText("2")
  await expect(detail).toContainText("$1.00")
  await expect(detail).toContainText("2 of 4 verified")
  const row = page.getByTestId(`run-row-${runs[0].id}`)
  await expect(row).toContainText("Discovery")
})

test("AC-1.4: closing changes nothing, the same day does not reopen it, the next day shows the next idea without a run", async ({
  page,
  request,
}) => {
  await page.unroute("**/api/discovery/daily*")
  await pinDay(page, DAY1)
  await page.goto("/today")
  const sheet = page.getByTestId("daily-suggestion-sheet")
  await expect(sheet).toBeVisible({ timeout: 15_000 })
  await expect(sheet).toContainText("Fesenjan")

  await page.keyboard.press("Escape")
  await expect(sheet).toBeHidden()
  const pending = await suggestions(request, "pending")
  expect(pending.find((s) => s.name === "Fesenjan")?.status).toBe("pending")

  // Reopening Today the same day neither shows the sheet nor asks for it.
  const asked: string[] = []
  page.on("request", (r) => {
    if (r.url().includes("/api/discovery/daily")) asked.push(r.url())
  })
  await page.reload()
  await page.waitForLoadState("networkidle")
  await expect(sheet).toHaveCount(0)
  expect(asked).toEqual([])

  // The next day the pool answers: no new run.
  const before = (await discoveryRuns(request)).length
  await pinDay(page, DAY2)
  await page.reload()
  await expect(sheet).toBeVisible({ timeout: 15_000 })
  await expect(sheet).toContainText("Buttered onion spaghetti")
  expect(await discoveryRuns(request)).toHaveLength(before)
})

test("AC-1.5: without a provider Today shows no sheet and asks for nothing, and the library has no Discover entry", async ({
  page,
}) => {
  await mockSettingValues(page, { "ai.provider": "" })
  const asked: string[] = []
  page.on("request", (r) => {
    if (r.url().includes("/api/discovery/daily")) asked.push(r.url())
  })
  await pinDay(page, localDay(2))
  await page.goto("/today")
  await page.waitForLoadState("networkidle")
  await expect(page.getByTestId("daily-suggestion-sheet")).toHaveCount(0)
  expect(asked).toEqual([])

  await page.goto("/recipes")
  await page.getByTestId("add-recipe").click()
  await expect(
    page.getByRole("menuitem", { name: "Import from a link" })
  ).toBeVisible()
  await expect(
    page.getByRole("menuitem", { name: "Discover recipes" })
  ).toHaveCount(0)
})

test("AC-3.2: Use what I know ranks the ideas by known share", async ({
  page,
  request,
}) => {
  const sheet = await openDiscover(page)
  const modes = sheet.getByRole("group", { name: "How to search" })
  for (const name of ["Daily idea", "Explore a cuisine", "Use what I know"]) {
    await expect(modes.getByRole("button", { name })).toBeVisible()
  }

  // Pasta (every ingredient known) ranks above Fesenjan, although the
  // research lists Fesenjan first.
  await modes.getByRole("button", { name: "Use what I know" }).click()
  await sheet.getByTestId("discover-focus").fill("chickpeas")
  await sheet.getByTestId("discover-start").click()
  const results = sheet.getByTestId("discover-results")
  await expect(results).toBeVisible({ timeout: 15_000 })
  await expect(results.getByRole("heading", { level: 3 })).toHaveText([
    "Buttered onion spaghetti",
    "Fesenjan",
  ])
  const runs = await discoveryRuns(request)
  expect(runs[0].scope).toMatchObject({ mode: "known", focus: "chickpeas" })
})

test("AC-3.1, AC-3.3: Explore a cuisine shows three cards; Not for me rejects with a reason and the URL never returns", async ({
  page,
  request,
}) => {
  const sheet = await openDiscover(page)
  const before = (await discoveryRuns(request)).length
  await sheet.getByRole("button", { name: "Explore a cuisine" }).click()
  await sheet.getByTestId("discover-cuisine").fill("Thai")
  await sheet.getByTestId("discover-start").click()
  const cards = sheet.getByTestId("discover-results").getByRole("listitem")
  await expect(cards).toHaveCount(3, { timeout: 15_000 })
  let runs = await discoveryRuns(request)
  expect(runs).toHaveLength(before + 1)
  expect(runs[0].scope).toMatchObject({ mode: "explore", cuisine: "thai" })

  await cards.first().getByRole("button", { name: "Not for me" }).click()
  await cards.first().getByRole("button", { name: "Takes too long" }).click()
  await expect(cards).toHaveCount(2)
  const rejected = (await suggestions(request, "rejected")).find(
    (s) => s.reason === "too_long"
  )
  expect(rejected?.source_url).toContain("thai=")

  // The same research again: every URL is a known one, so nothing is new.
  await sheet.getByTestId("discover-start").click()
  await expect(sheet.getByText(/No recipe passed the checks/)).toBeVisible({
    timeout: 15_000,
  })
  runs = await discoveryRuns(request)
  expect(runs[0].notes).toMatch(/^0 of 3 verified; discarded: /)
  const all = [
    ...(await suggestions(request, "pending")),
    ...(await suggestions(request, "rejected")),
  ]
  expect(all.filter((s) => s.source_url === rejected!.source_url)).toHaveLength(
    1
  )
})

test("AC-3.4: with a knowledge-only model the ideas carry no Search-grounded badge and no searches are billed", async ({
  page,
  request,
}) => {
  const knowledge = await useDiscoveryScript({ webSearch: false })
  extraScripts.push(knowledge)
  const sheet = await openDiscover(page)
  await sheet.getByRole("button", { name: "Explore a cuisine" }).click()
  await sheet.getByTestId("discover-cuisine").fill("Thai")
  await sheet.getByTestId("discover-start").click()
  const cards = sheet.getByTestId("suggestion-card")
  await expect(cards).toHaveCount(3, { timeout: 15_000 })
  await expect(sheet.getByTestId("suggestion-grounded")).toHaveCount(0)

  const runs = await discoveryRuns(request)
  expect(runs[0]).toMatchObject({ status: "ok", web_search_requests: 0 })
  const stored = (await suggestions(request, "pending")).filter((s) =>
    s.source_url.includes(knowledge.run)
  )
  expect(stored).toHaveLength(3)
  expect(stored.every((s) => !s.grounded)).toBe(true)
})

test("AC-3.5: the guided state offers Discover recipes only with a provider", async ({
  page,
}) => {
  await page.route("**/api/setup", (route) =>
    route.fulfill({
      json: { completed: true, recipe_count: 3, recipe_target: 10 },
    })
  )
  await page.goto("/recipes")
  const guide = page.getByTestId("recipe-onboarding")
  await expect(guide.getByTestId("onboarding-discover")).toBeVisible()
  await expect(
    guide.getByRole("heading", { name: "Discover recipes" })
  ).toBeVisible()

  await mockSettingValues(page, { "ai.provider": "" })
  await page.reload()
  await expect(guide).toBeVisible()
  await expect(guide.getByTestId("onboarding-import")).toBeVisible()
  await expect(guide.getByTestId("onboarding-create")).toBeVisible()
  await expect(guide.getByTestId("onboarding-discover")).toHaveCount(0)
})

test("AC-2: Add to my recipes opens a prefilled review; saving adds the recipe and accepts the idea", async ({
  page,
  request,
}) => {
  const idea = (await suggestions(request, "pending")).find(
    (s) => s.name === "Fesenjan" && s.source_url.includes("r=")
  )!
  expect(idea).toBeTruthy()

  await page.unroute("**/api/discovery/daily*")
  await pinDay(page, DAY1)
  await page.goto("/today")
  const sheet = page.getByTestId("daily-suggestion-sheet")
  await expect(sheet).toContainText("Fesenjan", { timeout: 15_000 })
  await sheet.getByRole("button", { name: "Add to my recipes" }).click()

  // Every row is matched to a catalogue leaf (Sumac and Pomegranate molasses
  // to the two unused ones), so Save is enabled without a single edit.
  const review = page.getByTestId("import-sheet")
  await expect(
    review.getByRole("heading", { name: "Review recipe" })
  ).toBeVisible()
  await expect(review.getByText("Sumac").first()).toBeVisible()
  await expect(review.getByText("Pomegranate molasses").first()).toBeVisible()
  const save = review.getByRole("button", { name: "Save recipe" })
  await expect(save).toBeEnabled({ timeout: 10_000 })

  const created = page.waitForResponse(
    (r) => /\/api\/foods$/.test(r.url()) && r.request().method() === "POST"
  )
  await save.click()
  const res = await created
  expect(res.ok(), await res.text()).toBeTruthy()
  const { id: foodId } = (await res.json()) as { id: number }

  // The recipe: source, explore tag, the suggestion's image, and the card's
  // numbers (both come from the same leaves).
  const food = (await (
    await request.get(`${API}/api/foods/${foodId}`)
  ).json()) as {
    name: string
    source_url: string
    tags: string[]
    image_path: string | null
  }
  expect(food.name).toBe("Fesenjan")
  expect(food.source_url).toBe(idea.source_url)
  expect(food.tags).toContain("explore")
  expect(food.image_path).toBeTruthy()
  const macros = (await (
    await request.get(`${API}/api/foods/${foodId}/nutrition`)
  ).json()) as { kcal: number; protein: number; fat: number; carbs: number }
  for (const k of ["kcal", "protein", "fat", "carbs"] as const) {
    expect(macros[k], k).toBeCloseTo(idea.per_portion[k], 1)
  }

  // The idea is accepted with the food, and its run's outcome follows.
  await expect
    .poll(
      async () =>
        (await suggestions(request, "accepted")).find((s) => s.id === idea.id)
          ?.food_id
    )
    .toBe(foodId)
  const run = (await (
    await request.get(`${API}/api/ai/runs/${idea.run_id}`)
  ).json()) as Run
  expect(run.outcome).toBe("accepted")

  // Insights: the discovery block counts the accepted recipe.
  await page.goto("/settings/assistant/insights")
  const block = page.getByTestId("insights-discovery")
  await expect(block).toBeVisible()
  await expect(block.getByTestId("discovery-accepted")).toContainText("1")
  await expect(block.getByTestId("discovery-rate")).not.toContainText("–")
  await expect(block.getByTestId("discovery-cost")).toContainText(/^.*\$\d/)
})
