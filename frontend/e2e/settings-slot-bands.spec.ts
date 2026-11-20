import { API, apiRequest, expect, test } from "./helpers"

async function bands() {
  const ctx = await apiRequest.newContext({ baseURL: API })
  const res = await ctx.get("/api/settings/slots/bands")
  const body = (await res.json()) as {
    items: { slot_id: number; min_pct: number; max_pct: number }[]
  }
  await ctx.dispose()
  return body.items
}

test.describe("Slot bands settings", () => {
  test("Settings → Meals explains the bands and refuses an invalid set with the numbers", async ({
    page,
  }) => {
    await page.goto("/settings/meals")
    const editor = page.getByTestId("slot-bands")
    await expect(editor).toBeVisible()
    await expect(editor).toContainText(/share of the day's energy/i)
    await expect(
      editor.getByRole("button", { name: /lunch is the biggest meal/i })
    ).toBeVisible()
    await expect(
      editor.getByRole("button", { name: /dinner is the biggest meal/i })
    ).toBeVisible()

    const first = (await bands())[0]
    const minInputs = editor.locator('input[aria-label$="Min %"]')
    const maxInputs = editor.locator('input[aria-label$="Max %"]')
    await maxInputs.first().fill("100")
    await minInputs.first().fill("100")

    const alert = editor.getByRole("alert")
    await expect(alert).toContainText(/at most 100/)
    await expect(
      editor.getByRole("button", { name: /save bands/i })
    ).toBeDisabled()

    // Nothing was stored.
    expect((await bands())[0]).toEqual(first)
  })
})
