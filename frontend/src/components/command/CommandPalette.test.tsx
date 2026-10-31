import { describe, expect, test, vi } from "vitest"
import { screen } from "@testing-library/react"

import { renderWithRouter } from "@/test/render"
import { CommandPalette } from "./CommandPalette"

vi.mock("@/lib/api/presets", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/presets")>()),
  getPresets: vi.fn().mockResolvedValue({ items: [], total: 0 }),
}))

describe("CommandPalette", () => {
  test("keeps the routes that left the nav reachable", async () => {
    renderWithRouter(
      <CommandPalette
        open
        onOpenChange={vi.fn()}
        onOpenCopyWeek={vi.fn()}
        defaultTargetDate="2026-10-26"
      />
    )

    for (const name of ["Ingredients", "Open Presets library"]) {
      expect(
        await screen.findByRole("option", { name: new RegExp(`^${name}$`) })
      ).toBeInTheDocument()
    }
  })
})
