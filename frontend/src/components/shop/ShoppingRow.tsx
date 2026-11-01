import { useQueryClient } from "@tanstack/react-query"
import { ChevronDown, MoreHorizontal } from "lucide-react"
import { useState } from "react"
import { useTranslation } from "react-i18next"

import { BatchBadge } from "@/components/batch/BatchBadge"
import { DraftTag } from "@/components/batch/DraftTag"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import type { Food } from "@/lib/api/foods"
import {
  SHOP_SECTIONS,
  type ShopSection,
  type ShoppingListItem,
} from "@/lib/api/shopping"
import type { BatchColour } from "@/lib/plan/batch-colours"
import { formatDate, weekdayShort } from "@/lib/plan/format"
import { useFood, useSetShopSection } from "@/lib/queries/foods"
import { foodKeys } from "@/lib/queries/keys"
import { useUndoToast } from "@/lib/queries/plan"
import { cn } from "@/lib/utils"

import { quantityText } from "./CopyAsText"

/** A batch the list can badge: its colour on the board and its portions. */
export interface ShopBatch {
  colour: BatchColour
  portions: number
}

/** What a row needs to label its "used in" entries. */
export interface UsedInContext {
  /** Batches of the viewed plan window, by session id. */
  batches: ReadonlyMap<number, ShopBatch>
  /** A slot's display name, or null for a slot the window doesn't show. */
  slotName: (slotId: number) => string | null
}

interface ShoppingRowProps {
  item: ShoppingListItem
  section: ShopSection
  ticked: boolean
  onToggle: (foodId: number) => void
  usedIn: UsedInContext
  /** In-store mode: larger type and rows. */
  large?: boolean
}

/**
 * One thing to buy (UX §5.7): a 44 px checkbox with the name and the
 * quantity in shopping units, an expandable "used in" list with batch badges,
 * and a menu that moves the item to another store section. A ticked row is
 * struck through; the section list moves it to the bottom.
 */
export function ShoppingRow({
  item,
  section,
  ticked,
  onToggle,
  usedIn,
  large = false,
}: ShoppingRowProps) {
  const { t, i18n } = useTranslation()
  const [open, setOpen] = useState(false)
  const checkboxId = `shop-item-${item.food_id}`
  const usesId = `shop-uses-${item.food_id}`
  // Only on the list because of draft plates ("Include draft plates").
  const fromDrafts =
    item.used_in.length > 0 && item.used_in.every((u) => u.draft)

  return (
    <li
      data-testid="shop-row"
      data-food-id={item.food_id}
      data-ticked={ticked || undefined}
      className="border-b border-line last:border-b-0"
    >
      <div className="flex items-center gap-1">
        <label
          htmlFor={checkboxId}
          className={cn(
            "flex min-w-0 flex-1 cursor-pointer items-center gap-3 py-1",
            large ? "min-h-14 text-h3" : "min-h-11 text-body"
          )}
        >
          <span className="grid size-11 shrink-0 place-items-center">
            <input
              id={checkboxId}
              type="checkbox"
              checked={ticked}
              onChange={() => onToggle(item.food_id)}
              className={cn(
                "cursor-pointer accent-accent dark:scheme-dark",
                large ? "size-6" : "size-5"
              )}
            />
          </span>
          <span
            className={cn(
              "min-w-0 flex-1 [overflow-wrap:anywhere] text-ink transition-colors duration-150 ease-out",
              ticked && "text-ink-muted line-through"
            )}
          >
            {item.name}
            {fromDrafts && (
              <>
                {" "}
                <DraftTag />
              </>
            )}
          </span>
          <span
            className={cn(
              "shrink-0 text-ink-muted tabular-nums",
              ticked && "line-through"
            )}
            data-testid="shop-quantity"
          >
            {quantityText(item, i18n)}
          </span>
        </label>
        {item.used_in.length > 0 && (
          <Button
            variant="ghost"
            aria-expanded={open}
            aria-controls={usesId}
            aria-label={t("shop.used_in_toggle", { name: item.name })}
            onClick={() => setOpen((v) => !v)}
            className="size-11 shrink-0 text-ink-muted md:pointer-fine:size-9"
          >
            <ChevronDown
              strokeWidth={1.5}
              aria-hidden="true"
              className={cn(
                "transition-transform duration-150 ease-out",
                open && "rotate-180"
              )}
            />
          </Button>
        )}
        <RowMenu item={item} section={section} />
      </div>
      {open && (
        <div id={usesId} className="flex flex-col gap-1 pb-3 pl-14">
          <p className="text-caption text-ink-muted">{t("shop.used_in")}</p>
          <ul className="flex flex-col gap-1.5">
            {item.used_in.map((use, i) => {
              const slot = usedIn.slotName(use.slot_id)
              const weekday = weekdayShort(use.date, i18n)
              const batch =
                use.session_id !== null
                  ? usedIn.batches.get(use.session_id)
                  : undefined
              return (
                <li
                  key={`${use.session_id ?? "leaf"}-${use.date}-${use.slot_id}-${i}`}
                  data-testid="shop-used-in"
                  className="flex flex-wrap items-center gap-x-2 gap-y-1 text-small text-ink"
                >
                  <span>
                    {t("shop.used_in_entry", {
                      name: use.food_name,
                      cell: slot ? t("batch.cell", { weekday, slot }) : weekday,
                    })}
                  </span>
                  <time
                    dateTime={use.date}
                    className="text-ink-muted tabular-nums"
                  >
                    {formatDate(use.date, i18n, {
                      day: "numeric",
                      month: "short",
                    })}
                  </time>
                  {batch && use.session_id !== null && (
                    <BatchBadge
                      variant="cook"
                      portions={batch.portions}
                      sessionId={use.session_id}
                      colour={batch.colour}
                    />
                  )}
                  {use.draft && !fromDrafts && <DraftTag />}
                </li>
              )
            })}
          </ul>
        </div>
      )}
    </li>
  )
}

/** "Move to section…": the nine sections in walk order, and "Reset to
 *  automatic" when the user has moved this item before. A move says where
 *  the item went, with Undo back to the old section (automatic again when
 *  it was automatic). */
function RowMenu({
  item,
  section,
}: {
  item: ShoppingListItem
  section: ShopSection
}) {
  const { t } = useTranslation()
  const qc = useQueryClient()
  const move = useSetShopSection()
  const offerUndo = useUndoToast()

  function moveTo(next: ShopSection) {
    const id = item.food_id
    // ResetSection loaded the food while the menu was open.
    const food = qc.getQueryData<Food>(foodKeys.detail(id))
    const wasAuto = food?.kind === "leaf" && food.shop_section_source === "auto"
    const back = wasAuto ? null : section
    move.mutate(
      { id, section: next },
      {
        onSuccess: () =>
          offerUndo(
            t("shop.moved", {
              name: item.name,
              section: t(`shop.section.${next}`),
            }),
            () => move.mutate({ id, section: back })
          ),
      }
    )
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          aria-label={t("shop.row_menu", { name: item.name })}
          className="size-11 shrink-0 text-ink-muted md:pointer-fine:size-9"
        >
          <MoreHorizontal strokeWidth={1.5} aria-hidden="true" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-52">
        <DropdownMenuLabel>{t("shop.move_to")}</DropdownMenuLabel>
        <DropdownMenuRadioGroup
          value={section}
          onValueChange={(next) => {
            if (next !== section) moveTo(next as ShopSection)
          }}
        >
          {SHOP_SECTIONS.map((s) => (
            <DropdownMenuRadioItem key={s} value={s}>
              {t(`shop.section.${s}`)}
            </DropdownMenuRadioItem>
          ))}
        </DropdownMenuRadioGroup>
        <ResetSection
          foodId={item.food_id}
          onReset={() => move.mutate({ id: item.food_id, section: null })}
        />
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

/** Mounted only while the menu is open, so the food is fetched on demand:
 *  the list does not say whether a section is automatic or the user's. */
function ResetSection({
  foodId,
  onReset,
}: {
  foodId: number
  onReset: () => void
}) {
  const { t } = useTranslation()
  const food = useFood(foodId)
  if (food.data?.kind !== "leaf" || food.data.shop_section_source !== "user")
    return null
  return (
    <>
      <DropdownMenuSeparator />
      <DropdownMenuItem onSelect={onReset}>
        {t("shop.reset_section")}
      </DropdownMenuItem>
    </>
  )
}
