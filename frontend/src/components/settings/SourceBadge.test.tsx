import { describe, expect, it } from "vitest"

import { renderWithRouter } from "@/test/render"
import { TooltipProvider } from "@/components/ui/tooltip"

import { SourceBadge } from "./SourceBadge"

function renderBadge(
  props: React.ComponentProps<typeof SourceBadge>
): ReturnType<typeof renderWithRouter> {
  return renderWithRouter(
    <TooltipProvider>
      <SourceBadge {...props} />
    </TooltipProvider>
  )
}

describe("SourceBadge", () => {
  it("renders a DB-variant for source=db", async () => {
    const { findByTestId } = renderBadge({ source: "db" })
    const el = await findByTestId("source-badge-db")
    expect(el).toBeTruthy()
    expect(el.textContent?.toLowerCase()).toContain("database")
  })

  it("renders an ENV-variant for source=env", async () => {
    const { findByTestId } = renderBadge({ source: "env" })
    const el = await findByTestId("source-badge-env")
    expect(el).toBeTruthy()
    expect(el.textContent?.toLowerCase()).toContain("env")
  })

  it("renders a DEFAULT-variant for source=default", async () => {
    const { findByTestId } = renderBadge({ source: "default" })
    const el = await findByTestId("source-badge-default")
    expect(el).toBeTruthy()
    expect(el.textContent?.toLowerCase()).toContain("default")
  })

  it.each([
    ["db", "Database"],
    ["env", "Env var"],
    ["default", "Default"],
  ] as const)(
    "labels source=%s in sentence case without letter-spacing",
    async (source, label) => {
      const { findByTestId } = renderBadge({ source })
      const el = await findByTestId(`source-badge-${source}`)
      expect(el).toHaveTextContent(new RegExp(`^${label}$`))
      expect(el.className).not.toMatch(/uppercase|tracking-/)
    }
  )

  it("uses Cookbook tokens only", async () => {
    const { findByTestId } = renderBadge({ source: "env" })
    const el = await findByTestId("source-badge-env")
    expect(el.className).not.toMatch(/text-primary|bg-primary/)
    expect(el.className).toMatch(/rounded-cell/)
  })

  it("uses a distinct test id per source so assertions stay precise", async () => {
    const { findByTestId, queryByTestId } = renderBadge({
      source: "db",
      envAlsoSet: true,
    })
    await findByTestId("source-badge-db")
    expect(queryByTestId("source-badge-env")).toBeNull()
  })
})
