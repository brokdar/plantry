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
    let deleted = false

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
      deleted = true
      await listRefetch

      // Preset ids are reused (no AUTOINCREMENT), so a parallel spec's new
      // preset can take this card's id: check the unique name instead.
      await expect(
        page
          .getByTestId(/^preset-card-/)
          .filter({ hasText: `Breakfast ${tag}` })
      ).toHaveCount(0)
    } finally {
      // Only if the test failed before the delete: the id may already
      // belong to another spec's preset.
      if (!deleted) await cleanupPreset(preset.id)
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
      await page.goto("/plan")

      // Cell is occupied — verify Rice visible.
      const cell = page.locator(`[data-cell-key="${today}|${slot.id}"]`)
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

      // Undo deletes the created plate, then restores the replaced one
      // through POST /api/plan/restore.
      const undoResp = page.waitForResponse(
        (r) =>
          /\/api\/plan\/restore$/.test(r.url()) &&
          r.request().method() === "POST"
      )
      await undoButton.click()
      const undone = await undoResp
      expect(undone.status()).toBe(200)

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

    // The board copies into its window start; the dialog's source defaults
    // to the previous window. Seed its first three days.
    const ctx0 = await apiRequest.newContext({ baseURL: API })
    const planRes = await ctx0.get("/api/plan")
    const { window: win } = (await planRes.json()) as {
      window: { start: string; prev_start: string }
    }
    await ctx0.dispose()
    const sourceStart = win.prev_start

    for (let i = 0; i < 3; i++) {
      await seedPlateWithComponent(shiftYMD(sourceStart, i), slot.id, {
        food_id: food.id,
        amount: 100,
        unit: "g",
      })
    }

    try {
      await page.goto("/plan")
      await page.getByRole("button", { name: "More plan actions" }).click()
      await page.getByRole("menuitem", { name: "Copy a previous week" }).click()

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

      // The window's first three days are now occupied.
      for (let i = 0; i < 3; i++) {
        await expect(
          page.locator(`[data-cell-key="${shiftYMD(win.start, i)}|${slot.id}"]`)
        ).toContainText(`Quinoa ${tag}`)
      }
    } finally {
      // Clean up plates from both source and target windows.
      const ctx = await apiRequest.newContext({ baseURL: API })
      const platesRes = await ctx.get(
        `/api/plates?from=${sourceStart}&to=${shiftYMD(win.start, 10)}`
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
