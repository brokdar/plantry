import { onlineManager } from "@tanstack/react-query"
import { act, screen } from "@testing-library/react"
import { afterEach, describe, expect, it } from "vitest"

import { renderWithRouter } from "@/test/render"

import { OfflineBanner } from "./OfflineBanner"

describe("OfflineBanner", () => {
  afterEach(() => {
    onlineManager.setOnline(true)
  })

  it("is silent online and says so when the connection drops and returns", async () => {
    renderWithRouter(<OfflineBanner />)
    const region = await screen.findByRole("status")
    expect(region).toBeEmptyDOMElement()

    act(() => onlineManager.setOnline(false))
    expect(region).toHaveTextContent(
      "You're offline. Changes can't be saved until you reconnect."
    )

    act(() => onlineManager.setOnline(true))
    expect(region).toBeEmptyDOMElement()
  })
})
