import {
  API,
  apiRequest,
  cleanupFood,
  cleanupSlot,
  expect,
  mockAnchorToday,
  seedLeafFood,
  seedPlateWithComponent,
  seedSlot,
  test,
  uid,
} from "./helpers"

// ── Preset API helpers ────────────────────────────────────────────────

interface SeededPreset {
  id: number
  name: string
}

async function seedPresetFromPlate(
  name: string,
  plateIds: number[],
  tags: string[] = []
): Promise<SeededPreset> {
  const ctx = await apiRequest.newContext({ baseURL: API })
  const res = await ctx.post("/api/presets", {
    data: { name, plate_ids: plateIds, tags },
  })
  const body = await res.json()
  expect(
    res.ok(),
    `seed preset "${name}" failed: ${res.status()} ${JSON.stringify(body)}`
  ).toBeTruthy()
  await ctx.dispose()
  return body as SeededPreset
}

async function cleanupPreset(id: number) {
  const ctx = await apiRequest.newContext({ baseURL: API })
  await ctx.delete(`/api/presets/${id}`)
  await ctx.dispose()
}

// ── Date helpers ──────────────────────────────────────────────────────

function todayYMD(): string {
  const d = new Date()
  const pad = (n: number) => String(n).padStart(2, "0")
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

function shiftYMD(ymd: string, days: number): string {
  const [y, m, d] = ymd.split("-").map(Number)
  const dt = new Date(y!, (m ?? 1) - 1, (d ?? 1) + days)
  const pad = (n: number) => String(n).padStart(2, "0")
  return `${dt.getFullYear()}-${pad(dt.getMonth() + 1)}-${pad(dt.getDate())}`
}

// ─────────────────────────────────────────────────────────────────────

test.describe("Presets", () => {
  test("creates a preset from a planner slot and sees it in the library", async ({
    page,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.dinner_${tag}`, "Moon", 950)
    const food = await seedLeafFood({ name: `Chicken ${tag}` })
    const today = todayYMD()
    const plate = await seedPlateWithComponent(today, slot.id, {
      food_id: food.id,
      amount: 150,
      unit: "g",
    })
    let presetId: number | undefined

    try {
      await mockAnchorToday(page)
      await page.goto("/")

      const cell = page.locator(`[data-testid="cell-0-${slot.id}"]`)
      await expect(cell).toBeVisible()
      await expect(cell.getByText(`Chicken ${tag}`)).toBeVisible()

      // Open the cell's overflow menu and click "Save as preset".
      await cell.hover()
      await cell.getByRole("button", { name: /actions/i }).click()
      await page.getByRole("menuitem", { name: /save as preset/i }).click()

      const dialog = page.getByTestId("save-preset-dialog")
      await expect(dialog).toBeVisible()

      const presetName = `Preset ${tag}`
      await dialog.getByLabel(/preset name/i).fill(presetName)

      const tagInput = dialog.getByTestId("save-preset-tag-input")
      await tagInput.fill("quick")
      await tagInput.press("Enter")

      const createResp = page.waitForResponse(
        (r) =>
          r.url().endsWith("/api/presets") && r.request().method() === "POST"
      )
      await dialog.getByRole("button", { name: /save preset/i }).click()
      const response = await createResp
      expect(response.status()).toBe(201)
      presetId = ((await response.json()) as { id: number }).id

      await page.goto("/presets")
      const card = page.locator(`[data-testid="preset-card-${presetId}"]`)
      await expect(card).toBeVisible()
      await expect(card.getByText(presetName)).toBeVisible()
      await expect(card.getByText("quick").first()).toBeVisible()
    } finally {
      if (presetId !== undefined) await cleanupPreset(presetId)
      // Plate may already be deleted by the preset/food cascade; ignore errors.
      const ctx = await apiRequest.newContext({ baseURL: API })
      await ctx.delete(`/api/plates/${plate.id}`)
      await ctx.dispose()
      await cleanupFood(food.id)
      await cleanupSlot(slot.id)
    }
  })

  test("applies a preset to an empty slot and the planner updates", async ({
    page,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.dinner_${tag}`, "Moon", 951)
    const food = await seedLeafFood({ name: `Rice ${tag}` })
    const today = todayYMD()

    // Seed a source plate, build a preset from it, then delete the plate so
    // today's slot is empty in the planner.
    const sourcePlate = await seedPlateWithComponent(today, slot.id, {
      food_id: food.id,
      amount: 100,
      unit: "g",
    })
    const preset = await seedPresetFromPlate(
      `Bowl ${tag}`,
      [sourcePlate.id],
      []
    )
    const ctx0 = await apiRequest.newContext({ baseURL: API })
    await ctx0.delete(`/api/plates/${sourcePlate.id}`)
    await ctx0.dispose()

    try {
      await mockAnchorToday(page)
      await page.goto("/")

      const cell = page.locator(`[data-testid="cell-0-${slot.id}"]`)
      await expect(cell).toBeVisible()

      // Hover to reveal the preset shortcut, then click it.
      await cell.hover()
      const presetTrigger = cell.getByTestId("slot-empty-preset-trigger")
      await presetTrigger.click()

      const picker = page.getByTestId("empty-slot-preset-picker")
      await expect(picker).toBeVisible()
      await expect(
        picker.getByRole("button", { name: new RegExp(`Bowl ${tag}`) })
      ).toBeVisible()

      const applyResp = page.waitForResponse(
        (r) =>
          new RegExp(`/api/presets/${preset.id}/apply$`).test(r.url()) &&
          r.request().method() === "POST"
      )
      await picker
        .getByRole("button", { name: new RegExp(`Bowl ${tag}`) })
        .click()
      const resp = await applyResp
      expect(resp.status()).toBe(200)

      // Slot now occupied — food name visible.
      await expect(cell.getByText(`Rice ${tag}`)).toBeVisible()
    } finally {
      await cleanupPreset(preset.id)
      // The applied plate references food; delete plates first by listing.
      const ctx = await apiRequest.newContext({ baseURL: API })
      const platesRes = await ctx.get(`/api/plates?from=${today}&to=${today}`)
      const body = (await platesRes.json()) as {
        plates?: { id: number; slot_id: number }[]
      }
      for (const p of body.plates ?? []) {
        if (p.slot_id === slot.id) {
          await ctx.delete(`/api/plates/${p.id}`)
        }
      }
      await ctx.dispose()
      await cleanupFood(food.id)
      await cleanupSlot(slot.id)
    }
  })

  test("deletes a preset from the library", async ({ page }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.dinner_${tag}`, "Moon", 952)
    const food = await seedLeafFood({ name: `Oats ${tag}` })
    const today = todayYMD()
    const sourcePlate = await seedPlateWithComponent(today, slot.id, {
      food_id: food.id,
      amount: 80,
      unit: "g",
    })
    const preset = await seedPresetFromPlate(`Breakfast ${tag}`, [
      sourcePlate.id,
    ])
    // Delete the source plate so cleanups are simple.
    const ctx0 = await apiRequest.newContext({ baseURL: API })
    await ctx0.delete(`/api/plates/${sourcePlate.id}`)
    await ctx0.dispose()

    try {
      await page.goto("/presets")
      const card = page.locator(`[data-testid="preset-card-${preset.id}"]`)
      await expect(card).toBeVisible()

      await card.getByRole("button", { name: /actions/i }).click()
      await page.getByRole("menuitem", { name: /delete preset/i }).click()

      const dialog = page.getByRole("dialog")
      await expect(dialog).toBeVisible()
      const deleteResp = page.waitForResponse(
        (r) =>
          new RegExp(`/api/presets/${preset.id}$`).test(r.url()) &&
          r.request().method() === "DELETE"
      )
      // The list refetch fires after delete (useDeletePreset invalidates the
      // lists). Wait for that refetch so the card-removal assertion isn't a
      // race against React Query under parallel-test load.
      const listRefetch = page.waitForResponse(
        (r) =>
          /\/api\/presets(\?|$)/.test(r.url()) &&
          r.request().method() === "GET" &&
          r.status() === 200
      )
      await dialog.getByRole("button", { name: /^delete$/i }).click()
      const resp = await deleteResp
      expect(resp.status()).toBe(204)
      await listRefetch

      await expect(
        page.locator(`[data-testid="preset-card-${preset.id}"]`)
      ).toHaveCount(0)
    } finally {
      // Idempotent cleanup in case the test failed before delete.
      await cleanupPreset(preset.id)
      await cleanupFood(food.id)
      await cleanupSlot(slot.id)
    }
  })

  test("apply with overwrite shows undo toast and undo restores the original plate", async ({
    page,
  }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.dinner_${tag}`, "Moon", 953)
    const pasta = await seedLeafFood({ name: `Pasta ${tag}` })
    const rice = await seedLeafFood({ name: `Rice ${tag}` })
    const today = todayYMD()

    // Seed a "source" plate with Pasta, build a preset, then delete the source.
    const srcPlate = await seedPlateWithComponent(today, slot.id, {
      food_id: pasta.id,
      amount: 120,
      unit: "g",
    })
    const preset = await seedPresetFromPlate(`Pasta Preset ${tag}`, [
      srcPlate.id,
    ])
    const ctx0 = await apiRequest.newContext({ baseURL: API })
    await ctx0.delete(`/api/plates/${srcPlate.id}`)
    await ctx0.dispose()

    // Seed the conflicting Rice plate on the same date+slot.
    await seedPlateWithComponent(today, slot.id, {
      food_id: rice.id,
      amount: 80,
      unit: "g",
    })

    try {
      await mockAnchorToday(page)
      await page.goto("/")

      // Cell is occupied — verify Rice visible.
      const cell = page.locator(`[data-testid="cell-0-${slot.id}"]`)
      await expect(cell).toBeVisible()
      await expect(cell.getByText(`Rice ${tag}`)).toBeVisible()

      // Use the command palette (⌘K) — its apply flow supports overwrite via
      // Shift+Enter (see preset.command.submit_skip_hint).
      await page.keyboard.press("ControlOrMeta+k")
      const palette = page.getByRole("dialog")
      await expect(palette).toBeVisible()
      await palette.locator("input").first().fill(`Pasta Preset ${tag}`)

      // Pick the preset row in the palette. Shift+Enter triggers overwrite.
      await page
        .getByRole("option", { name: new RegExp(`Pasta Preset ${tag}`) })
        .first()
        .click()
      // After selecting a preset, the palette typically reveals a target picker;
      // most apply flows for a planner-anchored palette pick today's date by
      // default. Trigger apply with overwrite via Shift+Enter.
      const applyResp = page.waitForResponse(
        (r) =>
          new RegExp(`/api/presets/${preset.id}/apply$`).test(r.url()) &&
          r.request().method() === "POST"
      )
      await page.keyboard.press("Shift+Enter")
      const applied = await applyResp
      expect(applied.status()).toBe(200)

      // Assert the Undo toast is visible immediately after apply, before the
      // planner refetch completes — this prevents racing Sonner's 4 s auto-dismiss
      // on slow CI machines.
      const undoButton = page.getByRole("button", { name: "Undo", exact: true })
      await expect(undoButton).toBeVisible()

      // Slot now shows Pasta (the preset content), not Rice.
      await expect(cell.getByText(`Pasta ${tag}`)).toBeVisible()

      // Undo button on the toast triggers POST /api/presets/undo-apply.
      const undoResp = page.waitForResponse(
        (r) =>
          /\/api\/presets\/undo-apply$/.test(r.url()) &&
          r.request().method() === "POST"
      )
      await undoButton.click()
      const undone = await undoResp
      expect(undone.status()).toBe(204)

      // After undo, Rice is restored.
      await expect(cell.getByText(`Rice ${tag}`)).toBeVisible()
    } finally {
      await cleanupPreset(preset.id)
      const ctx = await apiRequest.newContext({ baseURL: API })
      const platesRes = await ctx.get(`/api/plates?from=${today}&to=${today}`)
      const body = (await platesRes.json()) as {
        plates?: { id: number; slot_id: number }[]
      }
      for (const p of body.plates ?? []) {
        if (p.slot_id === slot.id) {
          await ctx.delete(`/api/plates/${p.id}`)
        }
      }
      await ctx.dispose()
      await cleanupFood(pasta.id)
      await cleanupFood(rice.id)
      await cleanupSlot(slot.id)
    }
  })

  test("copies a past week onto the current week", async ({ page }) => {
    const tag = uid()
    const slot = await seedSlot(`slot.dinner_${tag}`, "Moon", 954)
    const food = await seedLeafFood({ name: `Quinoa ${tag}` })

    // The planner anchors today; the CopyFromWeekDialog passes `from`
    // (anchor date) as `target_start`. Source defaults to `from - 7 days`.
    // Seed plates on `from-7`, `from-6`, `from-5` so they land on the visible
    // window when copied.
    const today = todayYMD()
    const sourceStart = shiftYMD(today, -7)

    for (let i = 0; i < 3; i++) {
      const date = shiftYMD(sourceStart, i)
      await seedPlateWithComponent(date, slot.id, {
        food_id: food.id,
        amount: 100,
        unit: "g",
      })
    }

    try {
      await mockAnchorToday(page)
      await page.goto("/")

      // Use the planner overflow menu (independent of empty-week state, which
      // other parallel tests may pollute by seeding plates on the current
      // window). The dialog's source defaults to `today - 7d`, exactly where
      // the seed lives.
      await page.getByTestId("planner-overflow").click()
      await page.getByTestId("copy-from-week-open").click()

      const dialog = page.getByTestId("copy-from-week-dialog")
      await expect(dialog).toBeVisible()

      const copyResp = page.waitForResponse(
        (r) =>
          /\/api\/presets\/copy-week$/.test(r.url()) &&
          r.request().method() === "POST"
      )
      await dialog.getByRole("button", { name: /^copy week$/i }).click()
      const resp = await copyResp
      expect(resp.status()).toBe(200)

      // All three target dates (today, today+1, today+2) are now occupied.
      const ctx = await apiRequest.newContext({ baseURL: API })
      const platesRes = await ctx.get(
        `/api/plates?from=${today}&to=${shiftYMD(today, 2)}`
      )
      const body = (await platesRes.json()) as {
        plates?: { date: string; slot_id: number }[]
      }
      await ctx.dispose()
      const matching = (body.plates ?? []).filter((p) => p.slot_id === slot.id)
      expect(matching.length).toBe(3)
    } finally {
      // Clean up plates from both source and target windows.
      const ctx = await apiRequest.newContext({ baseURL: API })
      const platesRes = await ctx.get(
        `/api/plates?from=${shiftYMD(sourceStart, -1)}&to=${shiftYMD(today, 10)}`
      )
      const body = (await platesRes.json()) as {
        plates?: { id: number; slot_id: number }[]
      }
      for (const p of body.plates ?? []) {
        if (p.slot_id === slot.id) {
          await ctx.delete(`/api/plates/${p.id}`)
        }
      }
      await ctx.dispose()
      await cleanupFood(food.id)
      await cleanupSlot(slot.id)
    }
  })
})
