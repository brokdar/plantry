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

export function FoodPlaceholder({
  category = "default",
  className,
  rounded = "xl",
  "aria-label": ariaLabel,
}: FoodPlaceholderProps) {
  const Icon = ICONS[category] ?? ICONS.default
  return (
    <div
      role="img"
      aria-label={ariaLabel ?? "food placeholder"}
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
        className="h-12 w-12 text-primary/40"
        strokeWidth={1.25}
      />
    </div>
  )
}
