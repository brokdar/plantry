import {
  Apple,
  Cake,
  CupSoda,
  Droplets,
  Leaf,
  Salad,
  Sandwich,
  Wheat,
  type LucideIcon,
} from "lucide-react"

import type { Food } from "@/lib/api/foods"
import { cn } from "@/lib/utils"

export type FoodPlaceholderCategory =
  | "main"
  | "side_starch"
  | "side_veg"
  | "side_protein"
  | "sauce"
  | "drink"
  | "dessert"
  | "standalone"
  | "ingredient"
  | "default"

type FoodPlaceholderProps = {
  category?: FoodPlaceholderCategory
  className?: string
  rounded?: "lg" | "xl" | "2xl" | "none"
  size?: "sm" | "md" | "lg"
  "aria-label"?: string
}

const ICONS: Record<FoodPlaceholderCategory, LucideIcon> = {
  main: Salad,
  side_starch: Wheat,
  side_veg: Leaf,
  side_protein: Sandwich,
  sauce: Droplets,
  drink: CupSoda,
  dessert: Cake,
  standalone: Salad,
  ingredient: Apple,
  default: Leaf,
}

const ROUNDED: Record<NonNullable<FoodPlaceholderProps["rounded"]>, string> = {
  lg: "rounded-lg",
  xl: "rounded-xl",
  "2xl": "rounded-2xl",
  none: "",
}

const ICON_SIZE: Record<NonNullable<FoodPlaceholderProps["size"]>, string> = {
  sm: "h-6 w-6",
  md: "h-12 w-12",
  lg: "h-16 w-16",
}

/**
 * categoryForFood picks the correct placeholder category from a food. Leaf
 * foods route to `"ingredient"` (the apple icon) instead of borrowing the
 * composed-food role taxonomy — a banana with no image used to render under
 * the "main course" salad icon, which is a small visual lie.
 */
export function categoryForFood(food: Food): FoodPlaceholderCategory {
  if (food.kind === "leaf") return "ingredient"
  return food.role ?? "main"
}

export function FoodPlaceholder({
  category = "default",
  className,
  rounded = "xl",
  size = "md",
  "aria-label": ariaLabel,
}: FoodPlaceholderProps) {
  const Icon = ICONS[category] ?? ICONS.default
  // When the caller doesn't provide an aria-label, treat the placeholder as
  // pure decoration: hide it from the accessibility tree so its text doesn't
  // pollute the accessible name of the surrounding button/list-item (which
  // already names the food). When an aria-label is given, expose it as an
  // image with that label for standalone use.
  const labelled = !!ariaLabel
  return (
    <div
      role={labelled ? "img" : "presentation"}
      aria-label={labelled ? ariaLabel : undefined}
      aria-hidden={labelled ? undefined : true}
      className={cn(
        "pointer-events-none flex items-center justify-center overflow-hidden",
        ROUNDED[rounded],
        className
      )}
      style={{
        backgroundImage:
          "linear-gradient(135deg, var(--surface-container-lowest) 0%, var(--primary-fixed) 100%)",
      }}
    >
      <Icon
        aria-hidden
        className={cn(ICON_SIZE[size], "text-primary/40")}
        strokeWidth={1.25}
      />
    </div>
  )
}
