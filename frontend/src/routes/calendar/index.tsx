import { addDays, addMonths, getDaysInMonth, subMonths } from "date-fns"
import { createFileRoute, useNavigate } from "@tanstack/react-router"
import { useDeferredValue, useMemo } from "react"
import { useTranslation } from "react-i18next"

import {
  CalendarHeader,
  type CalendarMode,
} from "@/components/calendar/CalendarHeader"
import type { PlannerDay } from "@/components/planner/PlannerGrid"
import { useSettings } from "@/lib/queries/settings"
import { usePlatesRange, usePlatesRangeInfinite } from "@/lib/queries/plates"
import { useTimeSlots } from "@/lib/queries/slots"
import { useFoods } from "@/lib/queries/foods"
import type { Food } from "@/lib/api/foods"

import { AgendaView } from "./-agenda"
import { MonthView } from "./-month"
import { WeekView } from "./-week"

export const Route = createFileRoute("/calendar/")({
  validateSearch: (
    search
  ): {
    mode: CalendarMode
    date?: string
    from?: string
    to?: string
    edit: boolean
    search: string
  } => ({
    mode: ((search.mode as string) ?? "month") as CalendarMode,
    date: (search.date as string) ?? undefined,
    from: (search.from as string) ?? undefined,
    to: (search.to as string) ?? undefined,
    edit: search.edit === "1" || search.edit === true || search.edit === "true",
    search: (search.search as string) ?? "",
  }),
  component: CalendarPage,
})

function padTwo(n: number): string {
  return String(n).padStart(2, "0")
}

function toYMD(d: Date): string {
  return `${d.getFullYear()}-${padTwo(d.getMonth() + 1)}-${padTwo(d.getDate())}`
}

function todayISO(): string {
  return toYMD(new Date())
}

function currentMonthISO(): string {
  const now = new Date()
  return `${now.getFullYear()}-${padTwo(now.getMonth() + 1)}`
}

function weekStartDate(weekStartsOn: 0 | 1 | 6): string {
  const today = new Date()
  const dow = today.getDay() // 0=Sun…6=Sat
  const diff = (dow - weekStartsOn + 7) % 7
  return toYMD(addDays(today, -diff))
}

/** Parse a YYYY-MM date string to { year, month (0-based) }. */
function parseYearMonth(s: string): { year: number; month: number } {
  const [y, m] = s.split("-").map(Number)
  return { year: y, month: (m ?? 1) - 1 }
}

/** Compute the visible grid range for a month view (partial leading/trailing weeks). */
function monthGridRange(
  year: number,
  month: number,
  weekStartsOn: 0 | 1 | 6
): { from: string; to: string } {
  const firstOfMonth = new Date(year, month, 1)
  const dow = firstOfMonth.getDay()
  const startDiff = (dow - weekStartsOn + 7) % 7
  const gridStart = addDays(firstOfMonth, -startDiff)
  const daysInMonth = getDaysInMonth(firstOfMonth)
  const totalGridDays = Math.ceil((startDiff + daysInMonth) / 7) * 7
  return {
    from: toYMD(gridStart),
    to: toYMD(addDays(gridStart, totalGridDays - 1)),
  }
}

function CalendarPage() {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()

  const { mode, date, to: toParam, edit, search } = Route.useSearch()

  const deferredSearch = useDeferredValue(search)

  // Settings
  const settingsQuery = useSettings()
  const settingValue = (key: string, fallback: string) =>
    settingsQuery.data?.items.find((i) => i.key === key)?.value ?? fallback

  const weekStartsOnStr = settingValue("plan.week_starts_on", "monday") as
    | "monday"
    | "sunday"
    | "saturday"
  const weekStartsOn: 0 | 1 | 6 =
    weekStartsOnStr === "sunday" ? 0 : weekStartsOnStr === "saturday" ? 6 : 1

  // Derive display date params with defaults
  const resolvedDate =
    date ?? (mode === "week" ? weekStartDate(weekStartsOn) : undefined)
  const resolvedMonth = date ?? currentMonthISO()

  // Month mode range
  const { year: monthYear, month: monthMonth } = parseYearMonth(resolvedMonth)
  const { from: monthFrom, to: monthTo } = monthGridRange(
    monthYear,
    monthMonth,
    weekStartsOn
  )

  // Week mode range: date is YYYY-MM-DD start of week
  const weekFrom = resolvedDate ?? weekStartDate(weekStartsOn)
  const weekTo = toYMD(addDays(new Date(weekFrom + "T00:00:00"), 6))

  // Agenda mode range: defaults to last 60 days
  const agendaAnchor = (() => {
    if (toParam) return toParam
    return todayISO()
  })()

  // Data fetching
  const monthPlatesQuery = usePlatesRange(
    mode === "month" ? monthFrom : "",
    mode === "month" ? monthTo : ""
  )
  const weekPlatesQuery = usePlatesRange(
    mode === "week" ? weekFrom : "",
    mode === "week" ? weekTo : ""
  )
  const agendaQuery = usePlatesRangeInfinite(
    mode === "agenda" ? agendaAnchor : ""
  )

  const slotsQuery = useTimeSlots(true)
  const slots = slotsQuery.data?.items ?? []

  const foodsQuery = useFoods({ limit: 200 })
  const foodsById = useMemo(() => {
    const map = new Map<number, Food>()
    for (const f of foodsQuery.data?.items ?? []) map.set(f.id, f)
    return map
  }, [foodsQuery.data])

  // Build PlannerDay[] for week mode
  const weekDays: PlannerDay[] = useMemo(() => {
    if (mode !== "week") return []
    const plates = weekPlatesQuery.data?.plates ?? []
    const start = new Date(weekFrom + "T00:00:00")
    return Array.from({ length: 7 }, (_, i) => {
      const d = addDays(start, i)
      const dateStr = toYMD(d)
      const weekday = (d.getDay() + 6) % 7 // 0=Mon…6=Sun
      return {
        date: dateStr,
        weekday,
        plates: plates.filter((p) => p.date === dateStr),
      }
    })
  }, [mode, weekFrom, weekPlatesQuery.data])

  // Header label
  const headerLabel = useMemo(() => {
    if (mode === "month") {
      return new Intl.DateTimeFormat(i18n.language, {
        month: "long",
        year: "numeric",
      }).format(new Date(monthYear, monthMonth))
    }
    if (mode === "week") {
      const from = new Date(weekFrom + "T00:00:00")
      const to = new Date(weekTo + "T00:00:00")
      const fmt = new Intl.DateTimeFormat(i18n.language, {
        month: "short",
        day: "numeric",
      })
      return `${fmt.format(from)} – ${fmt.format(to)}`
    }
    // agenda
    return t("calendar.mode_agenda")
  }, [mode, monthYear, monthMonth, weekFrom, weekTo, i18n.language, t])

  // Navigation handlers
  function handlePrev() {
    if (mode === "month") {
      const prev = subMonths(new Date(monthYear, monthMonth), 1)
      const newDate = `${prev.getFullYear()}-${padTwo(prev.getMonth() + 1)}`
      void navigate({
        to: "/calendar",
        search: (s) => ({ ...s, mode, date: newDate, edit, search }),
      })
    } else if (mode === "week") {
      const prev = addDays(new Date(weekFrom + "T00:00:00"), -7)
      void navigate({
        to: "/calendar",
        search: (s) => ({ ...s, mode, date: toYMD(prev), edit, search }),
      })
    } else {
      // agenda: shift back 60 days
      const newTo = toParam
        ? toYMD(addDays(new Date(toParam + "T00:00:00"), -60))
        : toYMD(addDays(new Date(), -60))
      void navigate({
        to: "/calendar",
        search: (s) => ({ ...s, mode, to: newTo, edit, search }),
      })
    }
  }

  function handleNext() {
    if (mode === "month") {
      const next = addMonths(new Date(monthYear, monthMonth), 1)
      const newDate = `${next.getFullYear()}-${padTwo(next.getMonth() + 1)}`
      void navigate({
        to: "/calendar",
        search: (s) => ({ ...s, mode, date: newDate, edit, search }),
      })
    } else if (mode === "week") {
      const next = addDays(new Date(weekFrom + "T00:00:00"), 7)
      void navigate({
        to: "/calendar",
        search: (s) => ({ ...s, mode, date: toYMD(next), edit, search }),
      })
    } else {
      const newTo = toParam
        ? toYMD(addDays(new Date(toParam + "T00:00:00"), 60))
        : toYMD(addDays(new Date(), 60))
      void navigate({
        to: "/calendar",
        search: (s) => ({ ...s, mode, to: newTo, edit, search }),
      })
    }
  }

  function handleToday() {
    if (mode === "month") {
      void navigate({
        to: "/calendar",
        search: (s) => ({ ...s, mode, date: currentMonthISO(), edit, search }),
      })
    } else if (mode === "week") {
      void navigate({
        to: "/calendar",
        search: (s) => ({
          ...s,
          mode,
          date: weekStartDate(weekStartsOn),
          edit,
          search,
        }),
      })
    } else {
      void navigate({
        to: "/calendar",
        search: (s) => ({
          ...s,
          mode,
          to: todayISO(),
          from: undefined,
          edit,
          search,
        }),
      })
    }
  }

  function handleModeChange(newMode: CalendarMode) {
    void navigate({
      to: "/calendar",
      search: (s) => ({
        ...s,
        mode: newMode,
        edit: false,
        search: "",
        date: undefined,
        from: undefined,
        to: undefined,
      }),
    })
  }

  function handleJumpToDate(pickedDate: string) {
    if (mode === "month") {
      // pickedDate is YYYY-MM-DD → extract YYYY-MM
      const newDate = pickedDate.slice(0, 7)
      void navigate({
        to: "/calendar",
        search: (s) => ({ ...s, mode, date: newDate, edit, search }),
      })
    } else if (mode === "week") {
      // Navigate to the week containing that date
      const d = new Date(pickedDate + "T00:00:00")
      const dow = d.getDay()
      const diff = (dow - weekStartsOn + 7) % 7
      const weekStart = addDays(d, -diff)
      void navigate({
        to: "/calendar",
        search: (s) => ({ ...s, mode, date: toYMD(weekStart), edit, search }),
      })
    } else {
      void navigate({
        to: "/calendar",
        search: (s) => ({ ...s, mode, to: pickedDate, edit, search }),
      })
    }
  }

  function handleSearchChange(newSearch: string) {
    void navigate({
      to: "/calendar",
      search: (s) => ({ ...s, mode, search: newSearch, edit }),
    })
  }

  function handleEditToggle() {
    void navigate({
      to: "/calendar",
      search: (s) => ({ ...s, mode, edit: !edit, search }),
    })
  }

  // Jump value for the header
  const jumpValue =
    mode === "month" ? resolvedMonth : mode === "week" ? weekFrom : agendaAnchor

  return (
    <div className="mx-auto max-w-7xl space-y-4 px-4 py-6 md:px-8 md:py-8">
      <h1 className="font-heading text-2xl font-bold tracking-tight text-on-surface md:text-3xl">
        {t("calendar.title")}
      </h1>

      <CalendarHeader
        mode={mode}
        label={headerLabel}
        onPrev={handlePrev}
        onNext={handleNext}
        onToday={handleToday}
        onModeChange={handleModeChange}
        search={search}
        onSearchChange={handleSearchChange}
        onJumpToDate={handleJumpToDate}
        jumpValue={jumpValue}
      />

      <div className="mt-4">
        {mode === "month" && (
          <MonthView
            year={monthYear}
            month={monthMonth}
            weekStartsOn={weekStartsOn}
            plates={monthPlatesQuery.data?.plates ?? []}
            search={deferredSearch}
            foodsById={foodsById}
          />
        )}

        {mode === "week" && (
          <WeekView
            days={weekDays}
            slots={slots}
            edit={edit}
            rangeFrom={weekFrom}
            rangeTo={weekTo}
            onEditToggle={handleEditToggle}
          />
        )}

        {mode === "agenda" && (
          <AgendaView
            data={agendaQuery.data}
            hasNextPage={agendaQuery.hasNextPage}
            isFetchingNextPage={agendaQuery.isFetchingNextPage}
            fetchNextPage={() => agendaQuery.fetchNextPage()}
            search={deferredSearch}
            weekStartsOn={weekStartsOn}
            foodsById={foodsById}
            slots={slots}
          />
        )}
      </div>
    </div>
  )
}
