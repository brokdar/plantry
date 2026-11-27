import { describe, expect, it } from "vitest"

import { mockSettingItem } from "@/test/fixtures"

import { chefPlanning, newDishesPerWeek } from "./chef-settings"

const items = (
  dishes: [string, "db" | "default"],
  chef: string,
  provider = ""
) => ({
  "chef.new_dishes_per_week": mockSettingItem(
    "chef.new_dishes_per_week",
    dishes[0],
    dishes[1]
  ),
  "chef.enabled": mockSettingItem("chef.enabled", chef, "db"),
  "ai.provider": mockSettingItem("ai.provider", provider, "db"),
})

describe("newDishesPerWeek (THEME-6)", () => {
  it("defaults to 1 with the Chef on and 0 with it off", () => {
    expect(newDishesPerWeek(items(["1", "default"], "true"))).toBe(1)
    expect(newDishesPerWeek(items(["1", "default"], "false"))).toBe(0)
  })
  it("keeps a stored value either way", () => {
    expect(newDishesPerWeek(items(["2", "db"], "false"))).toBe(2)
    expect(newDishesPerWeek(undefined)).toBe(0)
  })
  it("shows the Chef's planning parts only with the Chef on and a provider", () => {
    expect(chefPlanning(items(["1", "default"], "true", "anthropic"))).toBe(
      true
    )
    expect(chefPlanning(items(["1", "default"], "true"))).toBe(false)
    expect(chefPlanning(items(["1", "default"], "false", "anthropic"))).toBe(
      false
    )
  })
})
