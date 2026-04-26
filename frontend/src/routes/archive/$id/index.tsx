import { createFileRoute, useNavigate } from "@tanstack/react-router"

import { useWeek } from "@/lib/queries/weeks"

function isoWeekMonday(year: number, week: number): Date {
  const jan4 = new Date(Date.UTC(year, 0, 4))
  const dow = jan4.getUTCDay() || 7
  const monday = new Date(jan4)
  monday.setUTCDate(jan4.getUTCDate() - (dow - 1) + (week - 1) * 7)
  return monday
}

export const Route = createFileRoute("/archive/$id/")({
  component: ArchiveDetailRedirect,
})

function ArchiveDetailRedirect() {
  const { id } = Route.useParams()
  const numericId = Number(id)
  const navigate = useNavigate()
  const weekQuery = useWeek(Number.isNaN(numericId) ? 0 : numericId)

  // Once we have the week data, redirect to /calendar?mode=week&date=<monday>
  const week = weekQuery.data
  if (week) {
    const monday = isoWeekMonday(week.year, week.week_number)
    const pad = (n: number) => String(n).padStart(2, "0")
    const dateStr = `${monday.getUTCFullYear()}-${pad(monday.getUTCMonth() + 1)}-${pad(monday.getUTCDate())}`
    void navigate({
      to: "/calendar",
      search: { mode: "week", date: dateStr, edit: false, search: "" },
      replace: true,
    })
  }

  return null
}
