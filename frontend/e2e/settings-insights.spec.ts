import { rm, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"

import type { APIRequestContext, Page } from "@playwright/test"

import {
  clearPendingSuggestions,
  useDiscoveryScript,
} from "./discovery-fixture"
import { expect, test } from "./helpers"

import {
  API,
  cleanupSlot,
  futureWindow,
  seedRecipeLibrary,
  seedSlot,
  setSetting,
  uid,
} from "./helpers"

// AC-8 (F7-AC1, F7-AC3). Runs in ai-serial: it drives the fake provider
// (model fake-e2e, which has no built-in price) and writes the global
// ai.price_overrides setting.
test.describe.configure({ mode: "serial" })

interface RunsPage {
  items: { id: number; status: string }[]
}

async function latestChatRun(request: APIRequestContext) {
  const res = await request.get(`${API}/api/ai/runs?kind=chat&limit=1`)
  return ((await res.json()) as RunsPage).items[0]
}

/** One chat turn through the panel; returns its run id once the run has
 *  finished (leaving the page earlier would cancel it). */
async function chatOnce(
  page: Page,
  request: APIRequestContext
): Promise<number> {
  const before = (await latestChatRun(request))?.id ?? 0
  await page.goto("/plan")
  await page.getByRole("button", { name: "Ask assistant" }).click()
  await page
    .getByTestId("assistant-composer-input")
    .fill("plan my tuesday dinner")
  await page.getByTestId("assistant-composer-submit").click()
  let runId = 0
  await expect
    .poll(
      async () => {
        const run = await latestChatRun(request)
        if (!run || run.id <= before) return "none"
        runId = run.id
        return run.status
      },
      { timeout: 15_000 }
    )
    .toBe("ok")
  return runId
}

/** A chat script that adds one draft plate (create_plate), then ends. */
function createPlateScript(date: string, slotId: number): string {
  const text = "Done, it is planned."
  const usage = { input_tokens: 1000, output_tokens: 30 }
  const input = { date, slot_id: slotId }
  return JSON.stringify({
    turns: [
      {
        events: [
          { type: "message_start", payload: { model: "fake-e2e" } },
          {
            type: "tool_call_start",
            payload: { id: "tu_1", name: "create_plate" },
          },
          {
            type: "tool_call_delta",
            payload: { id: "tu_1", args_json: JSON.stringify(input) },
          },
        ],
        stop_reason: "tool_use",
        message: {
          content: [
            {
              type: "tool_use",
              tool_use_id: "tu_1",
              tool_name: "create_plate",
              tool_input: input,
            },
          ],
        },
        usage,
      },
      {
        events: [
          { type: "message_start", payload: { model: "fake-e2e" } },
          { type: "assistant_delta", payload: { text } },
        ],
        stop_reason: "end_turn",
        message: { content: [{ type: "text", text }] },
        usage,
      },
    ],
  })
}

test("a chat turn shows up in Insights with its steps and an unknown cost", async ({
  page,
  request,
}) => {
  const slot = await seedSlot(`slot.insights-${uid()}`, "moon", 98)
  const scriptPath = join(tmpdir(), `plantry-insights-${uid()}.json`)
  try {
    // AC-4.2: the chat run adds a plate, so its row counts one edit.
    const { days } = await futureWindow()
    await writeFile(scriptPath, createPlateScript(days[0], slot.id))
    await setSetting("ai.fake_script", scriptPath)
    const runId = await chatOnce(page, request)

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
    await expect(row.getByTestId("run-edits")).toHaveText("1")

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
      .filter({ hasText: "Added a plate" })
    await expect(tool).toBeVisible()
    await tool.getByText("Raw JSON").click()
    await expect(tool.getByText("Arguments")).toBeVisible()
  } finally {
    await request.delete(`${API}/api/settings/ai.fake_script`)
    await rm(scriptPath, { force: true })
    await cleanupSlot(slot.id)
  }
})

// Z5 §17.6: generate and regenerate runs are rows too, with version, tokens,
// cost, status, score and outcome; the chat and discovery rows are above and
// below. The heuristic calls no model, so its tokens are 0 and its cost is a
// plain zero rather than "unknown".
test("heuristic generate and regenerate runs are rows with version, score and outcome", async ({
  page,
  request,
}) => {
  const tag = uid()
  const lib = await seedRecipeLibrary(tag, 10)
  const { start, days } = await futureWindow()
  const runs = async (kind: string) =>
    (
      (await (
        await request.get(`${API}/api/ai/runs?kind=${kind}&limit=1`)
      ).json()) as {
        items: Record<string, unknown>[]
      }
    ).items[0]
  const finished = async (kind: string, after: number) => {
    let run: Record<string, unknown> | undefined
    await expect
      .poll(async () => {
        run = await runs(kind)
        return run && (run.id as number) > after ? run.status : "none"
      })
      .toBe("ok")
    return run!
  }
  try {
    const lastGen = ((await runs("generate"))?.id as number) ?? 0
    const gen = await request.post(`${API}/api/plan/generate`, {
      data: { start, chooser: "heuristic" },
    })
    expect(gen.status(), await gen.text()).toBe(202)
    const generated = await finished("generate", lastGen)

    const lastRegen = ((await runs("regenerate"))?.id as number) ?? 0
    const regen = await request.post(`${API}/api/plan/regenerate`, {
      data: {
        start,
        chooser: "heuristic",
        scope: { kind: "day", date: days[2] },
      },
    })
    expect(regen.status(), await regen.text()).toBe(202)
    const regenerated = await finished("regenerate", lastRegen)

    for (const run of [generated, regenerated]) {
      expect(run).toMatchObject({
        chooser: "heuristic",
        prompt_version: expect.stringMatching(/^h\d+$/),
        status: "ok",
        tokens: { input: 0, output: 0 },
      })
      expect(run.score).not.toBeNull()
    }

    // Accepting the week gives both runs an outcome.
    const accepted = await request.post(`${API}/api/plan/accept`, {
      data: { from: start, to: days[6] },
    })
    expect(accepted.ok(), await accepted.text()).toBeTruthy()

    await page.goto("/settings/assistant/insights")
    const g = page.getByTestId(`run-row-${generated.id}`)
    const r = page.getByTestId(`run-row-${regenerated.id}`)
    for (const [row, kind] of [
      [g, "Generate"],
      [r, "Regenerate"],
    ] as const) {
      const cells = row.getByRole("cell")
      await expect(cells.nth(1)).toHaveText(kind)
      await expect(cells.nth(3)).toHaveText("Without the assistant")
      await expect(cells.nth(4)).toHaveText(/^h\d+$/)
      await expect(cells.nth(5)).toHaveText("0")
      await expect(cells.nth(6)).toHaveText("$0.00")
      await expect(cells.nth(7)).toHaveText("Done")
      await expect(cells.nth(8)).toContainText("100% covered")
      await expect(cells.nth(9)).toHaveText("Accepted")
    }
  } finally {
    await request.delete(`${API}/api/plan?from=${start}&to=${days[6]}`)
    await lib.cleanup()
  }
})

// F4-AC5: a discovery run is a row of its own kind, its detail counts the web
// searches, and the discovery block and by-version table list it.
test("a discovery run shows up in Insights with its searches", async ({
  page,
  request,
}) => {
  const script = await useDiscoveryScript()
  try {
    const started = await request.post(`${API}/api/discovery`, {
      data: { mode: "explore", cuisine: "persian" },
    })
    expect(started.status(), await started.text()).toBe(202)
    const { run_id } = (await started.json()) as { run_id: number }
    await expect
      .poll(
        async () =>
          (
            (await (
              await request.get(`${API}/api/discovery/runs/${run_id}`)
            ).json()) as { status: string }
          ).status,
        { timeout: 15_000 }
      )
      .not.toBe("running")

    await page.goto("/settings/assistant/insights")
    const row = page.getByTestId(`run-row-${run_id}`)
    await expect(row).toContainText("Discovery")
    await expect(row).toContainText("Done")
    await expect(page.getByTestId("insights-discovery")).toBeVisible()
    await expect(
      page
        .getByRole("table", { name: "By prompt version" })
        .getByRole("row")
        .filter({ hasText: "Discovery" })
        .filter({ hasText: "d1" })
    ).toHaveCount(1)

    await row.getByRole("button").click()
    await expect(page.getByTestId("run-detail-searches")).toContainText("2")
  } finally {
    await clearPendingSuggestions()
    await script.remove()
  }
})

// DoD walk nit: at 375 the runs table shows only its first columns, so a
// hint says it scrolls sideways (the chat run above is in the table).
test("at 375 px the runs table says it scrolls sideways", async ({ page }) => {
  await page.setViewportSize({ width: 375, height: 812 })
  await page.goto("/settings/assistant/insights")
  const table = page.getByRole("table", { name: "Runs" })
  await expect(table).toBeVisible()
  // ScrollTable: the hint sits above the scroll box that holds the table.
  const box = table.locator("xpath=../..")
  await expect(box.getByTestId("scroll-hint")).toHaveText(
    "Scroll sideways for more columns"
  )
  // Scrolled to its end, the table hides nothing and the hint goes.
  await table.locator("xpath=..").evaluate((el) => {
    el.scrollLeft = el.scrollWidth
  })
  await expect(box.getByTestId("scroll-hint")).toHaveCount(0)
})

test("a price override re-prices the runs table", async ({ page, request }) => {
  const slot = await seedSlot(`slot.insights-${uid()}`, "moon", 98)
  try {
    const runId = await chatOnce(page, request)

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
    await expect(row.getByRole("cell").nth(6)).toHaveText(/^\$\d/)
  } finally {
    await request.delete(`${API}/api/settings/ai.price_overrides`)
    await cleanupSlot(slot.id)
  }
})
