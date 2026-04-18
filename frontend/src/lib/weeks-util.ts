import { getISOWeek, getISOWeekYear } from "date-fns"

export function currentYearWeek(date: Date = new Date()) {
  return { year: getISOWeekYear(date), week: getISOWeek(date) }
}
