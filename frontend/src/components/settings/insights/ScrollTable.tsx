import { ArrowRight } from "lucide-react"
import { useEffect, useRef, useState } from "react"
import { useTranslation } from "react-i18next"

import { cn } from "@/lib/utils"

/** A wide table's sideways scroll box. While columns are hidden to the
 *  right, a hint above it says so (on phones the table shows only its first
 *  columns, with no other sign that it scrolls). */
export function ScrollTable({
  children,
  className,
}: {
  children: React.ReactNode
  className?: string
}) {
  const { t } = useTranslation()
  const ref = useRef<HTMLDivElement>(null)
  const [more, setMore] = useState(false)

  useEffect(() => {
    const el = ref.current
    if (!el) return
    const check = () =>
      setMore(el.scrollWidth - el.clientWidth - el.scrollLeft > 1)
    const observer = new ResizeObserver(check)
    observer.observe(el)
    el.addEventListener("scroll", check, { passive: true })
    return () => {
      observer.disconnect()
      el.removeEventListener("scroll", check)
    }
  }, [])

  return (
    <div className={cn("flex flex-col gap-1", className)}>
      {more && (
        <p
          className="flex items-center justify-end gap-1 text-caption text-ink-muted"
          data-testid="scroll-hint"
        >
          {t("settings_page.insights.scroll_hint")}
          <ArrowRight className="size-3.5" strokeWidth={1.5} aria-hidden />
        </p>
      )}
      <div
        ref={ref}
        className="overflow-x-auto rounded-cell border border-line bg-panel"
      >
        {children}
      </div>
    </div>
  )
}
