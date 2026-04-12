import { useState } from "react"
import { useTranslation } from "react-i18next"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Button } from "@/components/ui/button"

interface BarcodeScannerModalProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onScan: (barcode: string) => void
}

export function BarcodeScannerModal({
  open,
  onOpenChange,
  onScan,
}: BarcodeScannerModalProps) {
  const { t } = useTranslation()
  const [barcode, setBarcode] = useState("")

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    const trimmed = barcode.trim()
    if (trimmed) {
      onScan(trimmed)
      setBarcode("")
      onOpenChange(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("lookup.enter_barcode")}</DialogTitle>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="space-y-4">
          <Input
            value={barcode}
            onChange={(e) => setBarcode(e.target.value)}
            placeholder="0123456789012"
            autoFocus
          />
          <Button type="submit" disabled={!barcode.trim()}>
            {t("lookup.scan_barcode")}
          </Button>
        </form>
      </DialogContent>
    </Dialog>
  )
}
