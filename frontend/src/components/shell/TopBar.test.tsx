import { describe, expect, test } from "vitest"
import { screen } from "@testing-library/react"

import { renderWithRouter } from "@/test/render"
import { TopBar } from "./TopBar"

describe("TopBar", () => {
  test("shows a settings gear only below 768 px", async () => {
    renderWithRouter(<TopBar />)
    const gear = await screen.findByRole("link", { name: /settings/i })
    expect(gear).toHaveAttribute("href", "/settings")
    // md (768 px) and up hides it; the rail carries Settings there.
    expect(gear).toHaveClass("md:hidden")
  })

  test("the whole bar is hidden from md up", async () => {
    renderWithRouter(<TopBar />)
    expect(await screen.findByTestId("topbar")).toHaveClass("md:hidden")
  })
})
