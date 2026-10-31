import { useTranslation } from "react-i18next"

const ROWS = 3
const DAYS = 7

/** The board's loading state: the grid's shape in sunken blocks (UX §6:
 *  skeletons, never a centred spinner). */
export function BoardSkeleton() {
  const { t } = useTranslation()
  return (
    <div
      role="status"
      aria-label={t("plan.board.loading")}
      data-testid="board-skeleton"
      className="flex flex-col gap-6"
    >
      <div className="flex flex-col gap-2">
        <div className="h-8 w-56 animate-pulse rounded-cell bg-panel-sunken" />
        <div className="h-4 w-40 animate-pulse rounded-cell bg-panel-sunken" />
      </div>
      <div className="grid grid-cols-[6rem_repeat(4,minmax(0,1fr))] gap-1 lg:grid-cols-[7rem_repeat(7,minmax(0,1fr))]">
        <div />
        {Array.from({ length: DAYS }, (_, d) => (
          <div
            key={d}
            className="h-10 animate-pulse rounded-cell bg-panel-sunken max-lg:[&:nth-child(n+6)]:hidden"
          />
        ))}
        {Array.from({ length: ROWS }, (_, r) => (
          <div key={r} className="contents">
            <div className="h-4 w-16 animate-pulse self-center rounded-cell bg-panel-sunken" />
            {Array.from({ length: DAYS }, (_, d) => (
              <div
                key={d}
                className={
                  "h-24 animate-pulse rounded-cell bg-panel-sunken" +
                  (d >= 4 ? " max-lg:hidden" : "")
                }
              />
            ))}
          </div>
        ))}
      </div>
    </div>
  )
}
