import { describe, expect, it } from "vitest"

import de from "./de.json"
import en from "./en.json"

// Plan §0.2 gate 5 (Z1-AC3): en and de have the same keys, and every key is
// used in src/.

type Tree = { [key: string]: string | Tree }

function flatten(tree: Tree, prefix = ""): string[] {
  return Object.entries(tree).flatMap(([k, v]) =>
    typeof v === "string" ? [prefix + k] : flatten(v, `${prefix}${k}.`)
  )
}

/** i18next plural forms resolve from the base key. */
const PLURAL = /_(zero|one|two|few|many|other)$/

/**
 * Keys built at runtime. Keep this minimal; each prefix says where it is
 * composed.
 */
const DYNAMIC_PREFIXES = [
  // backend `message_key` values (ApiError.messageKey, toastError)
  "error.",
  // slot names: the slot's `name_key` from the API (slot-label.ts)
  "slot.",
]

const sources = import.meta.glob<string>(
  ["/src/**/*.{ts,tsx}", "!/src/**/*.test.{ts,tsx}", "!/src/test/**"],
  { query: "?raw", import: "default", eager: true }
)
const code = Object.values(sources).join("\n")

/** Every string literal in the code, and the static heads of template
 *  literals (`plan.cell.${x}` → "plan.cell."). */
const literals = new Set<string>()
for (const m of code.matchAll(/["'`]([\w.-]+)["'`]/g)) literals.add(m[1])
const templateHeads = [...code.matchAll(/`([\w.-]+)\$\{/g)].map((m) => m[1])

function used(key: string): boolean {
  const base = key.replace(PLURAL, "")
  if (literals.has(key) || literals.has(base)) return true
  if (DYNAMIC_PREFIXES.some((p) => key.startsWith(p))) return true
  return templateHeads.some((head) => key.startsWith(head))
}

const enKeys = flatten(en as Tree)
const deKeys = flatten(de as Tree)

describe("i18n keys", () => {
  it("en and de define the same keys", () => {
    expect(deKeys.filter((k) => !enKeys.includes(k))).toEqual([])
    expect(enKeys.filter((k) => !deKeys.includes(k))).toEqual([])
  })

  it("every key is used in src/", () => {
    expect(enKeys.filter((k) => !used(k))).toEqual([])
  })
})
