import { readFile, rm, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"

import {
  API,
  apiRequest,
  cleanupFood,
  seedComposedFood,
  seedLeafFood,
  setSetting,
  uid,
} from "./helpers"

// Shared by the discovery specs: the "Explorer" fixture (a few
// recipes and two catalogue leaves no recipe uses), the fake research script
// with its {{SITE}} filled, and the recipe site started by playwright.config.

/** The lane's fixture site (e2e/fixtures/discovery-site). */
export const SITE = `http://127.0.0.1:${process.env.E2E_SITE_PORT ?? 9100}`

/** Local YYYY-MM-DD of `now` plus `n` days: what the page's clock reads. */
export function localDay(n = 0): string {
  const d = new Date()
  d.setDate(d.getDate() + n)
  return d.toLocaleDateString("sv")
}

// per 100 g
const KNOWN_LEAVES = [
  ["Chicken thighs", 200, 25, 10, 0],
  ["Walnut halves", 650, 15, 65, 14],
  ["Yellow onion", 40, 1, 0.1, 9],
  ["Basmati rice", 350, 8, 1, 78],
  ["Butter", 717, 1, 81, 0.1],
  ["Sea salt", 0, 0, 0, 0],
  ["Spaghetti", 350, 12, 2, 72],
] as const
const NEW_LEAVES = [
  ["Sumac", 300, 5, 10, 50],
  ["Pomegranate molasses", 280, 1, 0, 70],
] as const

/**
 * The Explorer catalogue: seven leaves used by one recipe, plus Sumac and
 * Pomegranate molasses that no recipe uses. `cleanup` removes the recipe, any
 * recipe saved from `source_url`s on the fixture site, then the leaves.
 */
export async function seedExplorer() {
  const leaves: Record<string, number> = {}
  for (const [name, kcal, protein, fat, carbs] of [
    ...KNOWN_LEAVES,
    ...NEW_LEAVES,
  ]) {
    const f = await seedLeafFood({
      name,
      kcal_100g: kcal,
      protein_100g: protein,
      fat_100g: fat,
      carbs_100g: carbs,
    })
    leaves[name] = f.id
  }
  const stew = await seedComposedFood({
    name: "Explorer pantry stew",
    role: "main",
    reference_portions: 4,
    children: KNOWN_LEAVES.map(([name], i) => ({
      child_id: leaves[name],
      amount: 100,
      unit: "g",
      grams: 100,
      sort_order: i,
    })),
  })
  return {
    leaves,
    stew,
    cleanup: async () => {
      const ctx = await apiRequest.newContext({ baseURL: API })
      const res = await ctx.get("/api/foods?tag=explore&limit=200")
      const { items } = (await res.json()) as {
        items: { id: number; source_url?: string | null }[]
      }
      await ctx.dispose()
      for (const f of items) {
        if (f.source_url?.startsWith(SITE)) await cleanupFood(f.id)
      }
      await cleanupFood(stew.id)
      for (const id of Object.values(leaves)) await cleanupFood(id)
    },
  }
}

/**
 * Fills chat-scripts/discovery.json ({{SITE}}, {{RUN}} makes this run's URLs
 * new), writes it to a path of its own (the fake client is cached per path)
 * and points `ai.fake_script` at it. `webSearch: false` is a knowledge-only
 * model: no search turns, no search fee.
 */
export async function useDiscoveryScript(opts: { webSearch?: boolean } = {}) {
  const run = uid()
  const raw = await readFile(
    join(process.cwd(), "e2e/fixtures/chat-scripts/discovery.json"),
    "utf8"
  )
  const script = JSON.parse(
    raw.replaceAll("{{SITE}}", SITE).replaceAll("{{RUN}}", run)
  ) as {
    capabilities: { web_search: boolean }
    turns: { stop_reason: string; web_search_requests?: number }[]
  }
  if (opts.webSearch === false) {
    script.capabilities.web_search = false
    script.turns = script.turns.filter((t) => t.stop_reason !== "pause_turn")
    for (const t of script.turns) {
      delete t.web_search_requests
    }
  }
  const path = join(tmpdir(), `plantry-discovery-${run}.json`)
  await writeFile(path, JSON.stringify(script))
  await setSetting("ai.fake_script", path)
  return {
    run,
    remove: async () => {
      const ctx = await apiRequest.newContext({ baseURL: API })
      await ctx.delete("/api/settings/ai.fake_script")
      await ctx.dispose()
      await rm(path, { force: true })
    },
  }
}

/** Rejects every pending suggestion, so a spec starts and ends with a clean
 *  pool (rejecting also drops their images). */
export async function clearPendingSuggestions() {
  const ctx = await apiRequest.newContext({ baseURL: API })
  const res = await ctx.get("/api/discovery/suggestions?status=pending")
  const { items } = (await res.json()) as { items: { id: number }[] }
  for (const s of items) {
    await ctx.post(`/api/discovery/suggestions/${s.id}/reject`, { data: {} })
  }
  await ctx.dispose()
}
