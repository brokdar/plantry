import { useNavigate } from "@tanstack/react-router"

import { MonthGrid } from "@/components/calendar/MonthGrid"
import type { Food } from "@/lib/api/foods"
import type { Plate } from "@/lib/api/plates"

interface MonthViewProps {
  year: number
  month: number // 0-based
  weekStartsOn: 0 | 1 | 6
  plates: Plate[]
  search: string
  foodsById?: Map<number, Food>
}

export function MonthView({
  year,
  month,
  weekStartsOn,
  plates,
  search,
  foodsById,
}: MonthViewProps) {
  const navigate = useNavigate()

  function handleCellClick(date: string) {
    void navigate({ to: "/day/$date", params: { date } })
  }

  return (
    <MonthGrid
      year={year}
      month={month}
      weekStartsOn={weekStartsOn}
      plates={plates}
      search={search}
      onCellClick={handleCellClick}
      foodsById={foodsById}
    />
  )
}
