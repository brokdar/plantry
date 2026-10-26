import { buttonVariants } from "./button"

const VARIANTS = [
  "default",
  "outline",
  "secondary",
  "ghost",
  "destructive",
  "link",
] as const

describe("buttonVariants", () => {
  it.each(VARIANTS)(
    "%s has no hover lift, sage shadow or focus ring",
    (variant) => {
      const classes = buttonVariants({ variant })
      expect(classes).not.toMatch(/hover:-?translate-y/)
      expect(classes).not.toContain("rgba(74,101,77")
      expect(classes).not.toContain("hover:shadow")
      expect(classes).not.toMatch(/focus-visible:ring/)
      // outline-none would suppress the global :focus-visible outline.
      expect(classes.split(" ")).not.toContain("outline-none")
    }
  )

  it("keeps the press feedback", () => {
    expect(buttonVariants()).toContain(
      "active:not-aria-[haspopup]:translate-y-px"
    )
  })
})
