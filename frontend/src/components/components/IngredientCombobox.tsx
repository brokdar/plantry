import { useState, useDeferredValue } from "react"
import { useTranslation } from "react-i18next"
import { Check, ChevronsUpDown, Plus, Search } from "lucide-react"
import { Link } from "@tanstack/react-router"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"
import { Skeleton } from "@/components/ui/skeleton"
import { useIngredients } from "@/lib/queries/ingredients"
import { cn } from "@/lib/utils"
import type { Ingredient } from "@/lib/api/ingredients"

type IngredientComboboxProps = {
  value: number
  selectedName?: string
  onSelect: (ingredient: Ingredient) => void
  disabled?: boolean
  testId?: string
}

export function IngredientCombobox({
  value,
  selectedName,
  onSelect,
  disabled,
  testId,
}: IngredientComboboxProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState("")
  const deferredSearch = useDeferredValue(search)

  const { data, isLoading } = useIngredients({
    search: deferredSearch || undefined,
    limit: 12,
  })

  const items = data?.items ?? []
  const hasSelection = value > 0

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="outline"
          role="combobox"
          aria-expanded={open}
          disabled={disabled}
          data-testid={testId}
          className={cn(
            "w-full justify-between",
            !hasSelection && "text-on-surface-variant"
          )}
        >
          <span className="truncate">
            {hasSelection
              ? (selectedName ?? `#${value}`)
              : t("component.select_ingredient")}
          </span>
          <ChevronsUpDown className="ml-2 size-4 shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent
        align="start"
        className="w-[var(--radix-popover-trigger-width)] gap-0 p-0"
      >
        <div className="flex items-center gap-2 border-b border-outline-variant/20 px-3 py-2">
          <Search
            className="size-4 shrink-0 text-on-surface-variant"
            aria-hidden
          />
          <Input
            autoFocus
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder={t("ingredient.search_placeholder")}
            className="border-0 bg-transparent px-0 shadow-none focus-visible:ring-0"
            data-testid={testId ? `${testId}-search` : undefined}
          />
        </div>
        <div className="max-h-72 overflow-y-auto p-1.5">
          {isLoading ? (
            <div className="space-y-1.5 p-1.5">
              {Array.from({ length: 4 }).map((_, i) => (
                <Skeleton key={i} className="h-10 w-full rounded-md" />
              ))}
            </div>
          ) : items.length === 0 ? (
            <p className="px-3 py-6 text-center text-xs text-on-surface-variant">
              {t("ingredient.no_results")}
            </p>
          ) : (
            <ul role="listbox" className="space-y-0.5">
              {items.map((item) => {
                const selected = item.id === value
                return (
                  <li key={item.id}>
                    <button
                      type="button"
                      role="option"
                      aria-selected={selected}
                      onClick={() => {
                        onSelect(item)
                        setSearch("")
                        setOpen(false)
                      }}
                      data-testid={
                        testId ? `${testId}-option-${item.id}` : undefined
                      }
                      className="flex w-full items-center gap-2 rounded-md px-2.5 py-2 text-left text-sm hover:bg-surface-container focus:bg-surface-container focus:outline-none"
                    >
                      <Check
                        className={cn(
                          "size-3.5 shrink-0",
                          selected ? "opacity-100" : "opacity-0"
                        )}
                      />
                      <div className="min-w-0 flex-1">
                        <p className="truncate font-medium text-on-surface">
                          {item.name}
                        </p>
                        <p className="truncate text-[11px] text-on-surface-variant">
                          {Math.round(item.kcal_100g)} kcal ·{" "}
                          {item.protein_100g.toFixed(0)}P ·{" "}
                          {item.carbs_100g.toFixed(0)}C ·{" "}
                          {item.fat_100g.toFixed(0)}F
                        </p>
                      </div>
                    </button>
                  </li>
                )
              })}
            </ul>
          )}
        </div>
        <div className="border-t border-outline-variant/20 px-1.5 py-1.5">
          <Button
            asChild
            variant="ghost"
            size="sm"
            className="w-full justify-start"
          >
            <Link
              to="/ingredients/new"
              target="_blank"
              data-testid={testId ? `${testId}-create` : undefined}
            >
              <Plus className="mr-1 size-4" />
              {t("ingredient.create")}
            </Link>
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  )
}
