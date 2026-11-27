import { writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"

import { request as apiRequest } from "@playwright/test"

import { API, expect, test, uid } from "./helpers"

// Taste (Chef D2): a scripted chat turn records "hates_<word>", the
// assistant shows the statement chips, [Always] makes it permanent, and the
// Settings → Taste page lists it with its source, flips it, removes it and
// Undoes the removal. Serial (ai-serial): the fake script is a global setting.
const letters = (n: string) => n.replace(/\d/g, (d) => "abcdefghij"[Number(d)])

function script(word: string): string {
  const turn = (stop: string, content: object[], events: object[]) => ({
    match_user: "taste",
    events,
    stop_reason: stop,
    message: { content },
    usage: { input_tokens: 900, output_tokens: 40 },
  })
  const start = {
    delay_ms: 10,
    type: "message_start",
    payload: { model: "fake-e2e" },
  }
  return JSON.stringify({
    turns: [
      turn(
        "tool_use",
        [
          {
            type: "tool_use",
            tool_use_id: "tu_taste",
            tool_name: "record_preference",
            tool_input: { key: `hates_${word}`, value: true },
          },
        ],
        [
          start,
          {
            delay_ms: 10,
            type: "tool_call_start",
            payload: { id: "tu_taste", name: "record_preference" },
          },
        ]
      ),
      turn(
        "end_turn",
        [{ type: "text", text: "Noted, no taste for it." }],
        [
          start,
          {
            delay_ms: 10,
            type: "assistant_delta",
            payload: { text: "Noted, no taste for it." },
          },
        ]
      ),
    ],
  })
}

test("a chat statement gets chips; the Taste page lists, flips, removes and Undoes it", async ({
  page,
}) => {
  const word = letters(uid())
  const path = join(tmpdir(), `plantry-taste-${word}.json`)
  await writeFile(path, script(word))
  const ctx = await apiRequest.newContext({ baseURL: API })
  try {
    const res = await ctx.put("/api/settings/ai.fake_script", {
      data: { value: path },
    })
    expect(res.ok(), await res.text()).toBeTruthy()

    await page.setViewportSize({ width: 1440, height: 900 })
    await page.goto("/plan")
    await page.getByRole("button", { name: "Ask assistant" }).click()
    const panel = page.getByTestId("assistant-panel")
    await page.getByTestId("assistant-composer-input").fill("taste: hate it")
    await page.getByTestId("assistant-composer-submit").click()
    await expect(
      panel.getByText("Noted, no taste for it.").first()
    ).toBeVisible({ timeout: 30_000 })

    // Without a tap it holds for this week only; [Always] makes it permanent.
    const chips = panel.getByTestId("assistant-statement")
    await expect(chips).toContainText(word)
    await expect(chips).toContainText("This holds for this week")
    await chips.getByTestId("assistant-statement-always").click()
    await expect(chips).toHaveAttribute("data-state", "always")

    await page.goto("/settings/taste")
    const rather = page.getByTestId("taste-rather_not")
    const row = rather.getByTestId("taste-item").filter({ hasText: word })
    await expect(row).toContainText(`You'd rather not have ${word}.`)
    await expect(row.getByTestId("taste-source")).toHaveText("You said so")

    await row.getByRole("button", { name: "Make it a love" }).click()
    const love = page
      .getByTestId("taste-love")
      .getByTestId("taste-item")
      .filter({ hasText: word })
    await expect(love).toContainText(`You love ${word}.`)
    await expect(love.getByTestId("taste-source")).toHaveText("You set this")

    await love.getByRole("button", { name: `Remove ${word}` }).click()
    await expect(
      page.getByTestId("taste-item").filter({ hasText: word })
    ).toHaveCount(0)
    await page.getByRole("button", { name: "Undo" }).first().click()
    await expect(
      page.getByTestId("taste-item").filter({ hasText: word })
    ).toHaveCount(1)
  } finally {
    const list = (await (await ctx.get("/api/taste")).json()) as {
      items: { id: number; key: string }[]
    }
    for (const i of list.items.filter((x) => x.key === word))
      await ctx.delete(`/api/taste/items/${i.id}`)
    await ctx.delete("/api/settings/ai.fake_script")
    await ctx.dispose()
  }
})

test("the Taste page is there without a key and says so when empty", async ({
  page,
}) => {
  await page.goto("/settings/taste")
  await expect(page.getByTestId("taste-page")).toBeVisible()
  await expect(page.getByRole("heading", { name: "Never" })).toBeVisible()
  await expect(
    page.getByRole("heading", { name: "Recently learned" })
  ).toBeVisible()
  await expect(page.getByRole("link", { name: "Open Kitchen" })).toBeVisible()
})
