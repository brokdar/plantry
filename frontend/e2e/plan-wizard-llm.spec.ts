import {
  addDays,
  API,
  expect,
  seedRecipeLibrary,
  setSetting,
  test,
  uid,
} from "./helpers"

// Phase 3 F3-AC3: the assistant fails (the fake provider's script breaks
// every call) and "Plan without the assistant" plans heuristically (D11:
// no automatic fallback). Runs in ai-serial: it swaps the global
// ai.fake_script.

const ERROR_SCRIPT =
  "../frontend/e2e/fixtures/chat-scripts/generate-week-error.json"

test("F3-AC3: an assistant failure offers Plan without the assistant", async ({
  page,
  request,
}) => {
  test.slow()
  const library = await seedRecipeLibrary(uid())
  const plan = await request.get(`${API}/api/plan`)
  const { window } = (await plan.json()) as { window: { start: string } }
  // Far beyond the windows the parallel specs use (futureWindow: ≤ 50).
  const start = addDays(window.start, 7 * 65)
  await setSetting("ai.fake_script", ERROR_SCRIPT)
  try {
    // The shared DB accumulates recipes, so pin the recipe gate open.
    await page.route("**/api/setup", (route) =>
      route.fulfill({
        json: { completed: true, recipe_count: 10, recipe_target: 10 },
      })
    )
    await page.setViewportSize({ width: 1440, height: 900 })
    await page.goto(`/plan/new?start=${start}`)
    const wizard = page.getByTestId("plan-wizard")
    await wizard.getByRole("button", { name: "Next" }).click()
    await wizard.getByRole("button", { name: "Skip" }).click()

    await expect(
      wizard.getByText(
        "The assistant didn't respond. Try again, or plan without the assistant."
      )
    ).toBeVisible({ timeout: 30_000 })

    const generated = page.waitForResponse(
      (r) =>
        r.url().includes("/api/plan/generate") &&
        r.request().method() === "POST"
    )
    await wizard
      .getByRole("button", { name: "Plan without the assistant" })
      .click()
    const res = await generated
    expect(res.request().postDataJSON()).toMatchObject({
      start,
      chooser: "heuristic",
    })
    const { run_id: runId } = (await res.json()) as { run_id: number }

    await expect(page.getByTestId("plan-wizard")).toHaveCount(0, {
      timeout: 30_000,
    })
    await expect(page.getByTestId("plan-summary")).toHaveText(/^Draft · /)
    const run = await request.get(`${API}/api/plan/runs/${runId}`)
    expect(await run.json()).toMatchObject({
      chooser: "heuristic",
      status: "ok",
    })
  } finally {
    await request.delete(`${API}/api/settings/ai.fake_script`)
    await request.delete(
      `${API}/api/plan?from=${start}&to=${addDays(start, 6)}`
    )
    await library.cleanup()
  }
})
