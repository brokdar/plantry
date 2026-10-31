import { expect, test } from "./helpers"

import { API, cleanupSlot, seedSlot, uid } from "./helpers"

// AC-8 (F7-AC1, F7-AC3). Runs in ai-serial: it drives the fake provider
// (model fake-e2e, which has no built-in price) and writes the global
// ai.price_overrides setting.
test.describe.configure({ mode: "serial" })

interface RunsPage {
  items: { id: number; kind: string }[]
}

async function chatOnce(page: import("@playwright/test").Page) {
  await page.goto("/plan")
  await page.getByRole("button", { name: "Ask assistant" }).click()
  await page.getByTestId("chat-composer-input").fill("plan my tuesday dinner")
  const chat = page.waitForResponse(
    (r) => r.url().includes("/api/ai/chat") && r.request().method() === "POST"
  )
  await page.getByTestId("chat-composer-submit").click()
  await chat
  await expect(page.getByTestId("chat-message-assistant").first()).toBeVisible({
    timeout: 10_000,
  })
}

test("a chat turn shows up in Insights with its steps and an unknown cost", async ({
  page,
  request,
}) => {
  const slot = await seedSlot(`slot.insights-${uid()}`, "moon", 98)
  try {
    await chatOnce(page)

    const res = await request.get(`${API}/api/ai/runs?kind=chat&limit=1`)
    const { items } = (await res.json()) as RunsPage
    const runId = items[0].id

    // Settings → Assistant → Insights (the static route beats $section).
    await page.goto("/settings/assistant")
    await page.getByTestId("insights-link").click()
    await expect(
      page.getByRole("heading", { level: 1, name: "Insights" })
    ).toBeVisible()

    const row = page.getByTestId(`run-row-${runId}`)
    await expect(row).toContainText("Chat")
    await expect(row).toContainText("fake-e2e")
    await expect(row).toContainText("cost unknown")
    await expect(row).toContainText("Done")

    await expect(page.getByTestId("spark-cost")).toContainText(
      /Excludes \d+ runs? without a price/
    )

    // The run's steps, in plain language, with raw JSON behind a disclosure.
    await row.getByRole("button").click()
    await expect(page).toHaveURL(new RegExp(`[?&]run=${runId}`))
    const sheet = page.getByTestId("run-detail-sheet")
    await expect(
      sheet
        .getByText(/^Asked the assistant \(fake-e2e, [\d,]+ tokens\)/)
        .first()
    ).toBeVisible()
    const tool = sheet
      .getByTestId("run-step")
      .filter({ hasText: "Checked your meal slots" })
    await expect(tool).toBeVisible()
    await tool.getByText("Raw JSON").click()
    await expect(tool.getByText("Arguments")).toBeVisible()
  } finally {
    await cleanupSlot(slot.id)
  }
})

test("a price override re-prices the runs table", async ({ page, request }) => {
  const slot = await seedSlot(`slot.insights-${uid()}`, "moon", 98)
  try {
    await chatOnce(page)
    const res = await request.get(`${API}/api/ai/runs?kind=chat&limit=1`)
    const runId = ((await res.json()) as RunsPage).items[0].id

    await page.goto("/settings/assistant")
    await expect(page.getByTestId("current-price")).toContainText(
      "fake-e2e has no built-in price"
    )
    await page.getByTestId("price-override-add").click()
    const override = page.getByTestId("price-override-row").last()
    await override.getByLabel("Model id").fill("fake-e2e")
    await override.getByLabel("Input").fill("1000")
    await override.getByLabel("Output").fill("1000")
    const saved = page.waitForResponse(
      (r) =>
        r.url().includes("/api/settings/ai.price_overrides") &&
        r.request().method() === "PUT"
    )
    await override.getByLabel("Output").blur()
    expect((await saved).ok()).toBe(true)
    await expect(page.getByTestId("current-price")).toContainText(
      "(your price)"
    )

    await page.getByTestId("insights-link").click()
    const row = page.getByTestId(`run-row-${runId}`)
    await expect(row).toBeVisible()
    await expect(row).not.toContainText("cost unknown")
    await expect(row.getByRole("cell").nth(5)).toHaveText(/^\$\d/)
  } finally {
    await request.delete(`${API}/api/settings/ai.price_overrides`)
    await cleanupSlot(slot.id)
  }
})
