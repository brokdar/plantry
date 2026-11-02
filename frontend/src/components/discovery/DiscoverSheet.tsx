import { useNavigate } from "@tanstack/react-router"
import { X } from "lucide-react"
import { useId, useState } from "react"
import { useTranslation } from "react-i18next"

import { useIsPhone } from "@/components/plan/use-is-phone"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetTitle,
} from "@/components/ui/sheet"
import type {
  DiscoveryMode,
  RejectReason,
  Suggestion,
} from "@/lib/api/discovery"
import { todayISO } from "@/lib/dates"
import {
  useDiscover,
  useDiscoveryRun,
  useRejectSuggestion,
  useSuggestions,
} from "@/lib/queries/discovery"
import { cn } from "@/lib/utils"

import { RejectReasons } from "./RejectReasons"
import { SuggestionCard } from "./SuggestionCard"

const MODES: DiscoveryMode[] = ["daily", "explore", "known"]
/** Cards shown per run (UX §5.8). */
const MAX_CARDS = 3

interface Search {
  runId: number
  mode: DiscoveryMode
}

/**
 * The Discover sheet over the library (`/recipes?add=discover`, UX §5.8):
 * pick a mode (a cuisine for Explore, an optional focus ingredient for Use
 * what I know), run one discovery, then up to 3 cards. The last results stay
 * while the library is open, so closing a review comes back to them.
 */
export function DiscoverSheet({
  open,
  onClose,
}: {
  open: boolean
  onClose: () => void
}) {
  const { t } = useTranslation()
  const phone = useIsPhone()
  const [mode, setMode] = useState<DiscoveryMode>("daily")
  const [cuisine, setCuisine] = useState("")
  const [focus, setFocus] = useState("")
  const [search, setSearch] = useState<Search | null>(null)
  const start = useDiscover()
  const run = useDiscoveryRun(search?.runId)
  const running = start.isPending || run.data?.status === "running"
  const ready = mode !== "explore" || cuisine.trim() !== ""

  function onSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!ready || running) return
    start.mutate(
      {
        mode,
        cuisine: mode === "explore" ? cuisine.trim() : undefined,
        focus: mode === "known" ? focus.trim() || undefined : undefined,
        today: todayISO(),
      },
      { onSuccess: ({ run_id }) => setSearch({ runId: run_id, mode }) }
    )
  }

  return (
    <Sheet open={open} onOpenChange={(next) => !next && onClose()}>
      <SheetContent
        side={phone ? "bottom" : "right"}
        showCloseButton={false}
        className={cn(
          "flex flex-col gap-0 bg-panel p-0 text-ink shadow-sheet",
          phone
            ? "max-h-[90dvh] rounded-t-sheet"
            : "w-full data-[side=right]:w-full sm:max-w-xl data-[side=right]:sm:max-w-xl"
        )}
        data-testid="discover-sheet"
      >
        <div className="flex items-start justify-between gap-2 border-b border-line px-4 pt-5 pb-3 md:px-6">
          <div className="flex flex-col gap-1">
            <SheetTitle className="font-heading text-h2 font-normal text-ink">
              {t("discovery.discover.title")}
            </SheetTitle>
            <SheetDescription className="text-small text-ink-muted">
              {t("discovery.discover.description")}
            </SheetDescription>
          </div>
          <Button
            variant="ghost"
            className="size-11 md:pointer-fine:size-8"
            aria-label={t("common.close")}
            onClick={onClose}
          >
            <X strokeWidth={1.5} aria-hidden />
          </Button>
        </div>

        <div className="flex-1 overflow-y-auto px-4 py-5 md:px-6">
          <form className="flex flex-col gap-4" onSubmit={onSubmit}>
            <div
              role="group"
              aria-label={t("discovery.discover.mode_label")}
              className="flex flex-wrap gap-2"
            >
              {MODES.map((m) => (
                <button
                  key={m}
                  type="button"
                  aria-pressed={mode === m}
                  onClick={() => setMode(m)}
                  className={cn(
                    "inline-flex min-h-11 items-center rounded-lg border px-3 text-small transition-colors duration-150 ease-out md:pointer-fine:min-h-8",
                    mode === m
                      ? "border-accent bg-accent font-semibold text-on-accent"
                      : "border-line bg-panel text-ink hover:border-ink-muted"
                  )}
                  data-testid={`discover-mode-${m}`}
                >
                  {t(`discovery.discover.mode_${m}`)}
                </button>
              ))}
            </div>
            {mode === "explore" && (
              <Field
                label={t("discovery.discover.cuisine_label")}
                placeholder={t("discovery.discover.cuisine_placeholder")}
                value={cuisine}
                onChange={setCuisine}
                testId="discover-cuisine"
              />
            )}
            {mode === "known" && (
              <Field
                label={t("discovery.discover.focus_label")}
                placeholder={t("discovery.discover.focus_placeholder")}
                value={focus}
                onChange={setFocus}
                testId="discover-focus"
              />
            )}
            <Button
              type="submit"
              disabled={!ready || running}
              className="h-11 self-start px-4 md:pointer-fine:h-9"
              data-testid="discover-start"
            >
              {t("discovery.discover.start")}
            </Button>
          </form>

          {search && <Results key={search.runId} search={search} />}
        </div>
      </SheetContent>
    </Sheet>
  )
}

function Field({
  label,
  placeholder,
  value,
  onChange,
  testId,
}: {
  label: string
  placeholder: string
  value: string
  onChange: (value: string) => void
  testId: string
}) {
  const id = useId()
  return (
    <div className="flex max-w-sm flex-col gap-2">
      <Label htmlFor={id}>{label}</Label>
      <Input
        id={id}
        value={value}
        placeholder={placeholder}
        maxLength={60}
        onChange={(e) => onChange(e.target.value)}
        className="h-11 md:pointer-fine:h-9"
        data-testid={testId}
      />
    </div>
  )
}

/** One run's progress, then its pending ideas. "Use what I know" ranks them
 *  by known-ingredient share; the other modes keep the run's order. */
function Results({ search }: { search: Search }) {
  const { t } = useTranslation()
  const run = useDiscoveryRun(search.runId)
  const status = run.data?.status
  const done = status === "ok" || status === "partial"
  const list = useSuggestions({ run_id: search.runId }, done)

  if (run.isError || status === "error") {
    return (
      <p role="alert" className="mt-6 text-body text-danger">
        {t(run.data?.error_key ?? "discovery.discover.failed")}
      </p>
    )
  }
  if (!done || !list.data) {
    return (
      <p role="status" className="mt-6 text-body text-ink-muted">
        {t("discovery.discover.running")}
      </p>
    )
  }

  const pending = list.data.items.filter((s) => s.status === "pending")
  if (search.mode === "known")
    pending.sort((a, b) => b.known_share - a.known_share)
  const cards = pending.slice(0, MAX_CARDS)

  if (cards.length === 0) {
    return (
      <p role="status" className="mt-6 text-body text-ink-muted">
        {t("discovery.discover.empty")}
      </p>
    )
  }
  return (
    <ul className="mt-6 flex flex-col gap-8" data-testid="discover-results">
      {cards.map((s) => (
        <li key={s.id} className="flex flex-col gap-4">
          <SuggestionCard suggestion={s} />
          <CardActions suggestion={s} />
        </li>
      ))}
    </ul>
  )
}

/** "Add to my recipes" swaps this sheet for the prefilled review (the
 *  `suggestion` param closes it). */
function CardActions({ suggestion }: { suggestion: Suggestion }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const reject = useRejectSuggestion()
  const [rejecting, setRejecting] = useState(false)

  if (rejecting) {
    return (
      <RejectReasons
        disabled={reject.isPending}
        onPick={(reason: RejectReason | undefined) =>
          reject.mutate({ id: suggestion.id, reason })
        }
      />
    )
  }
  return (
    <div className="flex flex-wrap gap-2 border-b border-line pb-6">
      <Button
        className="h-11 px-4 md:pointer-fine:h-9"
        onClick={() =>
          void navigate({
            to: "/recipes",
            search: { add: "discover", suggestion: suggestion.id },
          })
        }
      >
        {t("discovery.sheet.add")}
      </Button>
      <Button
        variant="outline"
        className="h-11 px-4 md:pointer-fine:h-9"
        onClick={() => setRejecting(true)}
      >
        {t("discovery.sheet.not_for_me")}
      </Button>
    </div>
  )
}
