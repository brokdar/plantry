import { useState, useDeferredValue } from "react"
import { useTranslation } from "react-i18next"
import { Search, ScanBarcode, Loader2 } from "lucide-react"
import { Input } from "@/components/ui/input"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { useLookup } from "@/lib/queries/lookup"
import type { LookupCandidate } from "@/lib/api/lookup"
import { BarcodeScannerModal } from "./BarcodeScannerModal"

interface LookupPanelProps {
  onSelect: (candidate: LookupCandidate) => void
}

export function LookupPanel({ onSelect }: LookupPanelProps) {
  const { t } = useTranslation()
  const [search, setSearch] = useState("")
  const [barcode, setBarcode] = useState("")
  const [scannerOpen, setScannerOpen] = useState(false)
  const deferredSearch = useDeferredValue(search)

  const { data, isLoading, isError } = useLookup(
    barcode
      ? { barcode }
      : { query: deferredSearch.length >= 2 ? deferredSearch : undefined }
  )

  function handleBarcodeScan(scanned: string) {
    setBarcode(scanned)
    setSearch("")
  }

  function handleSearchChange(value: string) {
    setSearch(value)
    setBarcode("")
  }

  const results = data?.results ?? []
  const recommendedIndex = data?.recommended_index ?? -1
  const hasQuery = !!(barcode || (deferredSearch && deferredSearch.length >= 2))

  return (
    <div className="space-y-4">
      <div className="flex gap-2">
        <div className="relative flex-1">
          <Search className="absolute top-2.5 left-3 size-4 text-muted-foreground" />
          <Input
            value={search}
            onChange={(e) => handleSearchChange(e.target.value)}
            placeholder={t("lookup.search_placeholder")}
            className="pl-9"
          />
        </div>
        <Button
          type="button"
          variant="outline"
          onClick={() => setScannerOpen(true)}
        >
          <ScanBarcode className="mr-2 size-4" />
          {t("lookup.scan_barcode")}
        </Button>
      </div>

      {isLoading && (
        <div className="flex items-center justify-center py-8">
          <Loader2 className="size-6 animate-spin text-muted-foreground" />
          <span className="ml-2 text-muted-foreground">
            {t("lookup.searching")}
          </span>
        </div>
      )}

      {isError && (
        <p className="py-4 text-center text-sm text-destructive">
          {t("error.ingredient.lookup_failed")}
        </p>
      )}

      {!isLoading && !isError && hasQuery && results.length === 0 && (
        <p className="py-8 text-center text-sm text-muted-foreground">
          {t("lookup.no_results")}
        </p>
      )}

      {!isLoading && results.length > 0 && (
        <div className="space-y-2">
          <p className="text-sm text-muted-foreground">
            {t("lookup.select_match")}
          </p>
          <div className="space-y-2">
            {results.map((candidate, index) => (
              <button
                key={`${candidate.source}-${candidate.name}-${index}`}
                type="button"
                className={`w-full rounded-lg border p-3 text-left transition-colors hover:bg-accent ${
                  index === recommendedIndex
                    ? "border-primary bg-primary/5"
                    : ""
                }`}
                onClick={() => onSelect(candidate)}
              >
                <div className="flex items-start justify-between gap-2">
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      <span className="font-medium">{candidate.name}</span>
                      {index === recommendedIndex && (
                        <Badge variant="secondary">
                          {t("lookup.recommended")}
                        </Badge>
                      )}
                    </div>
                    {candidate.kcal_100g != null && (
                      <p className="mt-1 text-sm text-muted-foreground">
                        {candidate.kcal_100g} kcal
                        {candidate.protein_100g != null &&
                          ` | P: ${candidate.protein_100g}g`}
                        {candidate.fat_100g != null &&
                          ` | F: ${candidate.fat_100g}g`}
                        {candidate.carbs_100g != null &&
                          ` | C: ${candidate.carbs_100g}g`}
                      </p>
                    )}
                  </div>
                  <Badge variant="outline">
                    {candidate.source === "off"
                      ? t("lookup.source_off")
                      : t("lookup.source_fdc")}
                  </Badge>
                </div>
              </button>
            ))}
          </div>
        </div>
      )}

      <BarcodeScannerModal
        open={scannerOpen}
        onOpenChange={setScannerOpen}
        onScan={handleBarcodeScan}
      />
    </div>
  )
}
