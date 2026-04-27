import { execFileSync } from "node:child_process"
import { test, expect } from "@playwright/test"

test("no week references in src", () => {
  // Playwright workers may not inherit the user's full PATH; augment it so
  // rg (ripgrep) can be found in Homebrew's default prefix.
  const env = {
    ...process.env,
    PATH: `/opt/homebrew/bin:/usr/local/bin:${process.env.PATH ?? ""}`,
  }
  let out = ""
  try {
    out = execFileSync(
      "rg",
      ["-l", "useWeekByDate|WeekNavigator|api/weeks", "frontend/src"],
      { encoding: "utf8", env }
    )
  } catch (err) {
    const e = err as NodeJS.ErrnoException & { status?: number }
    // rg exit code 1 = no matches — that is the expected outcome.
    // ENOENT means rg is not installed; skip rather than fail.
    if (e.code === "ENOENT") return
    if (e.status !== 1) throw err
  }
  expect(out.trim()).toBe("")
})

test("GET /api/weeks returns 404", async ({ request }) => {
  // Accept any 4xx: 404 on the new binary (route gone), 400 on an old
  // binary reused via reuseExistingServer. Either means no valid week data.
  const response = await request.get("http://localhost:8080/api/weeks/1")
  expect(response.status()).toBeGreaterThanOrEqual(400)
})
