import { useState, useDeferredValue } from "react"
import { useTranslation } from "react-i18next"
import { useRouterState } from "@tanstack/react-router"
import {
  Check,
  Database,
  Globe,
  Loader2,
  ScanBarcode,
  Search,
  Sparkles,
} from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { useDebugWorkflow } from "@/lib/debugWorkflow"
import { useLookup } from "@/lib/queries/lookup"
import type { LookupCandidate } from "@/lib/api/lookup"
import { cn } from "@/lib/utils"

import { BarcodeScannerModal } from "./BarcodeScannerModal"
import { LookupDebugPanel } from "./LookupDebugPanel"
import { NutritionDetail } from "./NutritionDetail"

interface LookupPanelProps {
  onSelect: (candidate: LookupCandidate) => void
}

/**
 * LookupPanel is the ingredient-resolution surface that sits inside the
 * new-ingredient editor. It combines a text query and barcode entry, shows
 * AI-ranked results in a compact list, previews the nutrition payload of the
 * selected candidate, and — when `?debug=true` is on the URL — surfaces the
 * full backend pipeline trace.
 */
export function LookupPanel({ onSelect }: LookupPanelProps) {
  const { t, i18n } = useTranslation()
  const [search, setSearch] = useState("")
  const [barcode, setBarcode] = useState("")
  const [scannerOpen, setScannerOpen] = useState(false)
  // User-chosen index; null means "follow the server's recommendation".
  const [userSelectedIndex, setUserSelectedIndex] = useState<number | null>(
    null
  )
  // Reset the user's pick when a new result set arrives — compared against
  // the data reference, which changes whenever the query or response does.
  const [lastDataRef, setLastDataRef] = useState<unknown>(null)
  const deferredSearch = useDeferredValue(search)

  // Debug flag — the settings toggle (localStorage-backed) is the primary
  // source; `?debug=true` on the URL is a per-session fallback, useful when
  // sharing reproducer links without touching a user's settings.
  const [debugPref] = useDebugWorkflow()
  const debugURL = useRouterState({
    select: (s) =>
      (s.location.search as { debug?: boolean } | undefined)?.debug === true,
  })
  const debug = debugPref || debugURL

  // Treat all-digit input of typical EAN/UPC length as a barcode so users can
  // paste or type a barcode into the same field. 8 = shortest valid GTIN.
  const typedBarcode = /^\d{8,}$/.test(deferredSearch.trim())
    ? deferredSearch.trim()
    : ""

  const lang = i18n.language?.slice(0, 2) || "en"
  const { data, isLoading, isError } = useLookup(
    barcode || typedBarcode
      ? { barcode: barcode || typedBarcode, lang, debug }
      : {
          query: deferredSearch.length >= 2 ? deferredSearch : undefined,
          lang,
          debug,
        }
  )

  const results = data?.results ?? []
  const recommendedIndex = data?.recommended_index ?? -1
  const hasQuery = !!(
    barcode ||
    typedBarcode ||
    (deferredSearch && deferredSearch.length >= 2)
  )
  const cameraAvailable =
    typeof navigator !== "undefined" && !!navigator.mediaDevices?.getUserMedia

  // Resetting derived state when inputs change — React's canonical pattern
  // (https://react.dev/reference/react/useState#storing-information-from-previous-renders).
  // Avoids an effect + setState cascade.
  if (data !== lastDataRef) {
    setLastDataRef(data)
    setUserSelectedIndex(null)
  }

  const effectiveIndex =
    userSelectedIndex ?? (recommendedIndex >= 0 ? recommendedIndex : 0)
  const selectedIndex = Math.min(
    Math.max(0, effectiveIndex),
    Math.max(0, results.length - 1)
  )
  const selected = results.length > 0 ? results[selectedIndex] : null

  function handleBarcodeScan(scanned: string) {
    setBarcode(scanned)
    setSearch("")
  }

  function handleSearchChange(value: string) {
    setSearch(value)
    setBarcode("")
  }

  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <div className="relative">
          <Search className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-on-surface-variant/60" />
          <Input
            value={search}
            onChange={(e) => handleSearchChange(e.target.value)}
            placeholder={t("lookup.search_placeholder")}
            className="pl-9"
          />
        </div>
        {cameraAvailable && (
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => setScannerOpen(true)}
            className="h-8 w-full gap-2 text-xs"
          >
            <ScanBarcode className="size-3.5" aria-hidden />
            {t("lookup.scan_barcode")}
          </Button>
        )}
      </div>

      {isLoading && (
        <div className="flex items-center justify-center gap-2 py-6 text-xs text-on-surface-variant">
          <Loader2 className="size-4 animate-spin" aria-hidden />
          {t("lookup.searching")}
        </div>
      )}

      {isError && (
        <p className="py-4 text-center text-xs text-destructive">
          {t("error.ingredient.lookup_failed")}
        </p>
      )}

      {!isLoading && !isError && hasQuery && results.length === 0 && (
        <p className="py-6 text-center text-xs text-on-surface-variant">
          {t("lookup.no_results")}
        </p>
      )}

      {!isLoading && results.length > 0 && (
        <div className="space-y-3">
          {results.length > 1 && (
            <div className="space-y-1.5">
              <p className="text-[10px] font-semibold tracking-[0.18em] text-on-surface-variant/70 uppercase">
                {t("lookup.candidates", { count: results.length })}
              </p>
              <ul
                className="max-h-44 space-y-0.5 overflow-y-auto rounded-xl bg-surface-container p-1"
                role="listbox"
              >
                {results.map((candidate, index) => {
                  const SourceIcon =
                    candidate.source === "off" ? Globe : Database
                  const isSelected = index === selectedIndex
                  const isRecommended = index === recommendedIndex
                  const key = `${candidate.source}-${candidate.fdc_id ?? candidate.barcode ?? candidate.name}-${index}`
                  return (
                    <li key={key}>
                      <button
                        type="button"
                        role="option"
                        aria-selected={isSelected}
                        onClick={() => setUserSelectedIndex(index)}
                        className={cn(
                          "flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left text-xs transition-colors",
                          isSelected
                            ? "bg-primary/10 text-on-surface"
                            : "text-on-surface-variant hover:bg-surface-container-high"
                        )}
                      >
                        <SourceIcon
                          className="size-3 shrink-0 opacity-70"
                          aria-hidden
                        />
                        <span className="min-w-0 flex-1 truncate">
                          {candidate.name}
                        </span>
                        {candidate.kcal_100g != null && (
                          <span className="shrink-0 font-mono text-[10px] tabular-nums opacity-60">
                            {Math.round(candidate.kcal_100g)} kcal
                          </span>
                        )}
                        {isRecommended && (
                          <Sparkles
                            className="size-3 shrink-0 text-primary"
                            aria-label={t("lookup.recommended")}
                          />
                        )}
                        {isSelected && (
                          <Check
                            className="size-3 shrink-0 text-primary"
                            aria-hidden
                          />
                        )}
                      </button>
                    </li>
                  )
                })}
              </ul>
            </div>
          )}

          {selected && (
            <div className="space-y-3">
              <NutritionDetail
                candidate={selected}
                recommended={selectedIndex === recommendedIndex}
              />
              <div className="flex items-center justify-between gap-2">
                <Badge variant="secondary" className="text-[10px] font-medium">
                  {selected.source === "off"
                    ? t("lookup.source_off")
                    : t("lookup.source_fdc")}
                </Badge>
                <Button
                  type="button"
                  size="sm"
                  onClick={() => onSelect(selected)}
                  className="h-8 gap-1.5 text-xs"
                >
                  <Check className="size-3.5" aria-hidden />
                  {t("lookup.apply")}
                </Button>
              </div>
            </div>
          )}
        </div>
      )}

      {debug && data?.trace && data.trace.length > 0 && (
        <LookupDebugPanel trace={data.trace} />
      )}

      <BarcodeScannerModal
        open={scannerOpen}
        onOpenChange={setScannerOpen}
        onScan={handleBarcodeScan}
      />
    </div>
  )
}
