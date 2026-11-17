import {
  API,
  expect,
  futureWindow,
  seedRecipeLibrary,
  test,
  uid,
} from "./helpers"

// AC-8 (F7-AC1, Z5 §17.6). A real (heuristic) generation plans every active
// slot of its window from the shared recipes, so it runs in `global-settings`,
// alone, not beside the parallel specs that seed and delete recipes and
// plates (it once ended `partial` in `ai-serial` next to them).
test.describe.configure({ mode: "serial" })

// Z5 §17.6: generate and regenerate runs are rows too, with version, tokens,
// cost, status, score and outcome (the chat and discovery rows are in
// settings-insights.spec.ts). The heuristic calls no model, so its tokens are
// 0 and its cost is a plain zero rather than "unknown".
test("heuristic generate and regenerate runs are rows with version, score and outcome", async ({
  page,
  request,
}) => {
  const tag = uid()
  // Hearty recipes (about 740 kcal a portion): the shared profile may carry a
  // kcal target whose H9 floor a day of light recipes cannot reach.
  const lib = await seedRecipeLibrary(tag, 10, undefined, 490)
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
