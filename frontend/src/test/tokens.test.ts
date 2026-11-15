import css from "../index.css?raw"

// The plan-board world (light, dark): green-biased neutrals, one green
// accent, six vivid batch line colours. [light, dark]
const TOKENS: Record<string, [string, string]> = {
  ground: ["#F5F6F4", "#111513"],
  panel: ["#FFFFFF", "#191E1B"],
  "panel-sunken": ["#EEF0EC", "#0D100E"],
  "panel-raised": ["#FFFFFF", "#212723"],
  ink: ["#18201B", "#E6EBE7"],
  "ink-muted": ["#5D6861", "#A0AAA3"],
  line: ["#E1E4DE", "#2A312D"],
  accent: ["#157A3C", "#4CC276"],
  "on-accent": ["#FFFFFF", "#0D100E"],
  "batch-1": ["#C2410C", "#F08A4B"],
  "batch-1-tint": ["#FFF1E8", "#3A2418"],
  "batch-2": ["#1D4ED8", "#7EA3F5"],
  "batch-2-tint": ["#E9EFFE", "#1C2640"],
  "batch-3": ["#6D28D9", "#A98BF5"],
  "batch-3-tint": ["#F2ECFE", "#2A2140"],
  "batch-4": ["#0F766E", "#3CC7B3"],
  "batch-4-tint": ["#E3F4F1", "#143330"],
  "batch-5": ["#BE185D", "#EF7FB0"],
  "batch-5-tint": ["#FCE8F1", "#3A1D2B"],
  "batch-6": ["#9D6006", "#E0B341"],
  "batch-6-tint": ["#FBF1DC", "#362C12"],
  success: ["#2E6B3F", "#6FCF8F"],
  warning: ["#A15C00", "#E3A54F"],
  danger: ["#B42318", "#F08B7B"],
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
      // Solid batch pills carry on-accent text; lane labels and outlined
      // pills carry the batch colour on ground, panel and its own tint.
      ...[1, 2, 3, 4, 5, 6].flatMap((n) => [
        ["on-accent", `batch-${n}`],
        [`batch-${n}`, "ground"],
        [`batch-${n}`, "panel"],
        [`batch-${n}`, `batch-${n}-tint`],
      ]),
      ["warning", "panel"],
    ])("%s on %s meets 4.5:1", (fg, bg) => {
      expect(contrast(declared(fg), declared(bg))).toBeGreaterThanOrEqual(4.5)
    })

    it.each(["accent", "success", "warning", "danger"])(
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
