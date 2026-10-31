import {
  BookOpen,
  CalendarDays,
  CookingPot,
  Package,
  Settings,
  ShoppingBasket,
} from "lucide-react"
import type { LucideIcon } from "lucide-react"

/** The palette's go-to entries: the four destinations plus Ingredients and
 *  Settings (plan F10). */
export const GO_TO_ENTRIES: {
  to: string
  labelKey: string
  icon: LucideIcon
}[] = [
  { to: "/today", labelKey: "nav.today", icon: CookingPot },
  { to: "/plan", labelKey: "nav.plan", icon: CalendarDays },
  { to: "/recipes", labelKey: "nav.recipes", icon: BookOpen },
  { to: "/recipes/ingredients", labelKey: "nav.ingredients", icon: Package },
  { to: "/shop", labelKey: "nav.shop", icon: ShoppingBasket },
  { to: "/settings", labelKey: "nav.settings", icon: Settings },
]
