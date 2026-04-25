import { ShoppingCart } from "lucide-react"
import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"

import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet"
import { useShoppingList } from "@/lib/queries/weeks"
import { cn } from "@/lib/utils"

interface ShoppingPanelProps {
  weekId: number
  open: boolean
  onOpenChange: (open: boolean) => void
}

function storageKey(weekId: number) {
  return `plantry:purchased:week:${weekId}`
}

function loadPurchased(weekId: number): Set<number> {
  try {
    const raw = localStorage.getItem(storageKey(weekId))
    if (!raw) return new Set()
    return new Set(JSON.parse(raw) as number[])
  } catch {
    return new Set()
  }
}

function savePurchased(weekId: number, ids: Set<number>) {
  localStorage.setItem(storageKey(weekId), JSON.stringify([...ids]))
}

export function ShoppingPanel({
  weekId,
  open,
  onOpenChange,
}: ShoppingPanelProps) {
  const { t } = useTranslation()
  const { data, isLoading } = useShoppingList(weekId)

  const [purchased, setPurchased] = useState<Set<number>>(() =>
    loadPurchased(weekId)
  )

  // Prune stale purchased IDs whenever the item list changes.

  useEffect(() => {
    if (!data) return
    const validIds = new Set(data.items.map((i) => i.food_id))
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setPurchased((prev) => {
      const pruned = new Set([...prev].filter((id) => validIds.has(id)))
      savePurchased(weekId, pruned)
      return pruned
    })
  }, [data, weekId])

  function toggle(id: number) {
    setPurchased((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      savePurchased(weekId, next)
      return next
    })
  }

  const items = data?.items ?? []

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side="right" className="flex w-full flex-col sm:max-w-sm">
        <SheetHeader>
          <SheetTitle className="flex items-center gap-2">
            <ShoppingCart className="h-4 w-4" />
            {t("shopping.title")}
          </SheetTitle>
          <SheetDescription>{t("shopping.description")}</SheetDescription>
        </SheetHeader>

        <div className="flex-1 overflow-y-auto py-4">
          {isLoading && (
            <p className="text-sm text-muted-foreground">
              {t("common.loading")}
            </p>
          )}

          {!isLoading && items.length === 0 && (
            <p className="text-sm text-muted-foreground">
              {t("shopping.empty")}
            </p>
          )}

          {items.length > 0 && (
            <ul className="space-y-2">
              {items.map((item) => {
                const checked = purchased.has(item.food_id)
                return (
                  <li
                    key={item.food_id}
                    className="flex items-center gap-3 rounded-md px-1 py-1.5 hover:bg-muted/50"
                  >
                    <input
                      type="checkbox"
                      id={`shop-${item.food_id}`}
                      checked={checked}
                      onChange={() => toggle(item.food_id)}
                      className="h-4 w-4 rounded border-input accent-primary"
                    />
                    <label
                      htmlFor={`shop-${item.food_id}`}
                      className={cn(
                        "flex flex-1 cursor-pointer items-center justify-between text-sm",
                        checked && "text-muted-foreground line-through"
                      )}
                    >
                      <span>{item.name}</span>
                      <span className="ml-2 text-xs text-muted-foreground tabular-nums">
                        {Math.round(item.total_grams)} g
                      </span>
                    </label>
                  </li>
                )
              })}
            </ul>
          )}
        </div>
      </SheetContent>
    </Sheet>
  )
}
