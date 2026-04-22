import { cn } from "@/lib/utils"

type EditorialAsideProps = {
  children: React.ReactNode
  className?: string
  sticky?: boolean
}

export function EditorialAside({
  children,
  className,
  sticky = true,
}: EditorialAsideProps) {
  return (
    <aside
      className={cn(
        "space-y-8",
        sticky && "lg:sticky lg:top-24 lg:self-start",
        className
      )}
      data-testid="editorial-aside"
    >
      {children}
    </aside>
  )
}

EditorialAside.Section = function EditorialAsideSection({
  children,
  className,
  tonal = "low",
}: {
  children: React.ReactNode
  className?: string
  tonal?: "low" | "lowest"
}) {
  return (
    <section
      className={cn(
        "space-y-6 rounded-xl p-8",
        tonal === "low"
          ? "bg-surface-container-low"
          : "editorial-shadow bg-surface-container-lowest",
        className
      )}
    >
      {children}
    </section>
  )
}
