import { useState } from "react"
import { useLocation } from "@tanstack/react-router"
import {
  BookOpen,
  CalendarDays,
  CookingPot,
  ShoppingBasket,
} from "lucide-react"
import type { LucideIcon } from "lucide-react"

import { todayISO } from "@/lib/planner-window"

export type PrimaryNavEntry = {
  id: "today" | "plan" | "recipes" | "shop"
  to: string
  labelKey: string
  icon: LucideIcon
  active: boolean
}

const RECIPES_PATH = /^\/(components|ingredients|import)(\/|$)/

/**
 * The four destinations (UX §2.1, plan D1), shared by the rail and the phone
 * tab bar. Today's date is read once at mount so render stays pure.
 */
export function usePrimaryNav(): PrimaryNavEntry[] {
  const [today] = useState(todayISO)
  const pathname = useLocation({ select: (l) => l.pathname })

  return [
    {
      id: "today",
      to: `/day/${today}`,
      labelKey: "nav.today",
      icon: CookingPot,
      active: pathname.startsWith("/day/"),
    },
    {
      id: "plan",
      to: "/",
      labelKey: "nav.plan",
      icon: CalendarDays,
      active: pathname === "/",
    },
    {
      id: "recipes",
      to: "/components",
      labelKey: "nav.recipes",
      icon: BookOpen,
      active: RECIPES_PATH.test(pathname),
    },
    {
      id: "shop",
      to: "/shop",
      labelKey: "nav.shop",
      icon: ShoppingBasket,
      active: pathname === "/shop" || pathname.startsWith("/shop/"),
    },
  ]
}
