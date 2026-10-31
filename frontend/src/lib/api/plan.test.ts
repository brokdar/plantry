import { describe, expect, it } from "vitest"

import { KNOWN_ISSUE_CODES, type KnownIssueCode } from "./plan"
import type { SessionWarningCode } from "./sessions"

describe("KNOWN_ISSUE_CODES", () => {
  it("lists the D22 codes and the H3/H4 session warning codes", () => {
    const session: (SessionWarningCode & KnownIssueCode)[] = [
      "leftover_before_cook",
      "past_keep_days",
      "deficit",
      "frozen_deficit",
    ]
    expect([...KNOWN_ISSUE_CODES].sort()).toEqual(
      [
        "missing_plate",
        "plate_in_none_cell",
        "too_many_cook_occasions",
        "diet_incompatible",
        "meat_days_exceeded",
        "diet_unknown",
        "variety_repeat",
        "variety_relaxed",
        ...session,
      ].sort()
    )
  })
})
