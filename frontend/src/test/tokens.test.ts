import css from "../index.css?raw"

// UX_DESIGN_SPEC §3.1 — "Cookbook with batch badges". [light, dark]
const TOKENS: Record<string, [string, string]> = {
  ground: ["#FBFBFA", "#161821"],
  panel: ["#FFFFFF", "#1D2030"],
  "panel-sunken": ["#F3F4F6", "#12141C"],
  "panel-raised": ["#FFFFFF", "#252A3F"],
  ink: ["#1C2233", "#E6E8F0"],
  "ink-muted": ["#5A6175", "#A3A8BB"],
  line: ["#E3E5EA", "#2E3245"],
  accent: ["#27407A", "#9DB2EA"],
  "on-accent": ["#FBFBFA", "#161821"],
  "batch-1": ["#7A3B2E", "#E29A88"],
  "batch-1-tint": ["#F3E7E2", "#35231F"],
  "batch-2": ["#8A6A12", "#D8B75A"],
  "batch-2-tint": ["#F4EEDB", "#332B14"],
  "batch-3": ["#2F6B5A", "#7CC4AD"],
  "batch-3-tint": ["#E1EFE9", "#1B302A"],
  "batch-4": ["#6A3D6E", "#C99BCD"],
  "batch-4-tint": ["#EFE4F0", "#2E2131"],
  "batch-5": ["#5D6B2A", "#B5C46E"],
  "batch-5-tint": ["#ECEFDC", "#272C16"],
  "batch-6": ["#3D5A80", "#93B0D6"],
  "batch-6-tint": ["#E3EAF3", "#1C2533"],
  success: ["#2E6B3F", "#79C48F"],
  warning: ["#9A5B00", "#E3A54F"],
  danger: ["#A23A2A", "#EE8B7B"],
}

// The shadcn theme names components/ui reads; index.css maps each onto a
// Cookbook token. (`accent` is shared with the Cookbook.)
const SHADCN = [
  "background",
  "foreground",
  "card",
  "card-foreground",
  "popover",
  "popover-foreground",
  "primary",
  "primary-foreground",
  "secondary",
  "secondary-foreground",
  "muted",
  "muted-foreground",
  "accent-foreground",
  "destructive",
  "border",
  "input",
  "ring",
]

const THEMES = [
  { name: "light", selector: ":root", index: 0 },
  { name: "dark", selector: ".dark", index: 1 },
] as const

/** Declarations of the first top-level rule whose selector is exactly `selector`. */
function block(selector: string): Record<string, string> {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")
  const match = new RegExp(`(?:^|\\n)${escaped}\\s*\\{([^}]*)\\}`).exec(css)
  if (!match) throw new Error(`no ${selector} block in index.css`)
  const body = match[1].replace(/\/\*[\s\S]*?\*\//g, "")
  const decls: Record<string, string> = {}
  for (const m of body.matchAll(/(--[\w-]+)\s*:\s*([^;]+);/g)) {
    decls[m[1]] = m[2].trim()
  }
  return decls
}

function luminance(hex: string): number {
  const n = hex.replace("#", "")
  const [r, g, b] = [0, 2, 4].map((i) => {
    const c = parseInt(n.slice(i, i + 2), 16) / 255
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4
  })
  return 0.2126 * r + 0.7152 * g + 0.0722 * b
}

function contrast(a: string, b: string): number {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x)
  return (hi + 0.05) / (lo + 0.05)
}

describe("design tokens (index.css)", () => {
  describe.each(THEMES)("$name theme", ({ selector, index }) => {
    const decls = block(selector)
    const hex = (token: string) => TOKENS[token][index]
    // Contrast is measured on what index.css actually declares.
    const declared = (token: string) => decls[`--${token}`] ?? "#000000"

    it.each(Object.keys(TOKENS))(
      "defines --%s with the spec value",
      (token) => {
        expect(decls[`--${token}`]?.toUpperCase()).toBe(hex(token))
      }
    )

    it.each([
      ["ink", "ground"],
      ["ink", "panel"],
      ["ink-muted", "ground"],
      ["ink-muted", "panel"],
      ["on-accent", "accent"],
      ...[1, 2, 3, 4, 5, 6].map((n) => ["ground", `batch-${n}`]),
    ])("%s on %s meets 4.5:1", (fg, bg) => {
      expect(contrast(declared(fg), declared(bg))).toBeGreaterThanOrEqual(4.5)
    })

    it.each(["success", "warning", "danger"])(
      "%s against ground meets 3:1",
      (state) => {
        expect(
          contrast(declared(state), declared("ground"))
        ).toBeGreaterThanOrEqual(3)
      }
    )
  })

  it("exposes the tokens as Tailwind colours", () => {
    const theme = block("@theme inline")
    for (const token of Object.keys(TOKENS)) {
      expect(theme[`--color-${token}`]).toBe(`var(--${token})`)
    }
  })

  it("turns slides into short fades under prefers-reduced-motion", () => {
    const match =
      /@media\s*\(prefers-reduced-motion:\s*reduce\)\s*\{([\s\S]*?)\n\}/.exec(
        css
      )
    expect(match, "no reduced-motion block").not.toBeNull()
    const body = match![1]
    expect(body).toMatch(/transition-duration:\s*0\.01ms\s*!important/)
    expect(body).toMatch(/animation-duration:\s*150ms\s*!important/)
    for (const v of [
      "--tw-enter-translate-x",
      "--tw-enter-translate-y",
      "--tw-exit-translate-x",
      "--tw-exit-translate-y",
    ]) {
      expect(body).toMatch(new RegExp(`${v}:\\s*0\\s*!important`))
    }
    // Spinners are exempt: they keep spinning, declared after the * rule.
    const spin = /\.animate-spin\s*\{([^}]*)\}/.exec(body)
    expect(spin, "no .animate-spin exemption").not.toBeNull()
    expect(spin!.index).toBeGreaterThan(body.indexOf("*,"))
    expect(spin![1]).toMatch(/animation-duration:\s*1s\s*!important/)
    expect(spin![1]).toMatch(
      /animation-iteration-count:\s*infinite\s*!important/
    )
  })

  it("gives every focusable element a 2px accent focus outline", () => {
    const match = /\n\s*:focus-visible\s*\{([^}]*)\}/.exec(css)
    expect(match, "no :focus-visible rule").not.toBeNull()
    expect(match![1]).toMatch(/outline:\s*2px solid var\(--accent\);/)
    expect(match![1]).toMatch(/outline-offset:\s*2px;/)
  })

  // Z1-AC4: index.css defines the Cookbook tokens, the shadcn theme names
  // that components/ui reads, and a few layout values, and nothing else. Any
  // deleted alias (Material, macro and chart palettes, AI accent, sidebar,
  // editorial shadow) coming back fails one of these allowlists.
  describe("defines no legacy alias", () => {
    const LAYOUT = ["shadow-soft", "radius", "tab-bar-h", "tab-bar-inset"]

    it.each(THEMES)(
      "$name block holds only Cookbook tokens",
      ({ selector }) => {
        const names = Object.keys(block(selector)).map((n) => n.slice(2))
        const allowed = [...Object.keys(TOKENS), ...LAYOUT]
        expect(names.filter((n) => !allowed.includes(n))).toEqual([])
      }
    )

    it("maps only the shadcn theme names onto the tokens", () => {
      const names = Object.keys(block(":root,\n.dark")).map((n) => n.slice(2))
      expect(names.sort()).toEqual([...SHADCN].sort())
    })

    it("exposes only Cookbook and shadcn colours to Tailwind", () => {
      const colours = Object.keys(block("@theme inline"))
        .filter((n) => n.startsWith("--color-"))
        .map((n) => n.slice("--color-".length))
      const allowed = [...Object.keys(TOKENS), ...SHADCN]
      expect(colours.filter((n) => !allowed.includes(n))).toEqual([])
    })

    it("keeps no Botanical utilities or keyframes", () => {
      const utilities = /@layer utilities\s*\{([\s\S]*?)\n\}/.exec(css)
      expect(utilities, "no utilities layer").not.toBeNull()
      const classes = new Set(
        [...utilities![1].matchAll(/\.([\w-]+)/g)].map((m) => m[1])
      )
      expect([...classes].sort()).toEqual(["font-heading", "hide-scrollbar"])
      // The assistant's change pulse (Phase 4 D17) is the one keyframe.
      expect(css.replace("@keyframes cell-pulse", "")).not.toMatch(
        /@keyframes|gradient\(/
      )
    })
  })

  it("no longer references Inter or Manrope", () => {
    expect(css).not.toMatch(/\bInter\b|Manrope|fontsource-variable\/inter/i)
  })
})
