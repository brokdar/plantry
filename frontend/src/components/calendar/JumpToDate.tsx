import { useState } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"
import { cn } from "@/lib/utils"

interface JumpToDateProps {
  value: string
  onSelect: (date: string) => void
  className?: string
}

export function JumpToDate({ value, onSelect, className }: JumpToDateProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)

  function handleChange(e: React.ChangeEvent<HTMLInputElement>) {
    const v = e.target.value
    if (v) {
      onSelect(v)
      setOpen(false)
    }
  }

  // Derive a display label from value (YYYY-MM or YYYY-MM-DD)
  const label = value || t("calendar.jump_to_date")

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          size="sm"
          className={cn(
            "border-outline-variant/60 bg-surface-container-lowest font-heading text-xs tracking-wide hover:bg-surface-container-low",
            className
          )}
        >
          {label}
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-auto p-3" align="start">
        <p className="mb-2 font-heading text-xs tracking-wide text-on-surface-variant">
          {t("calendar.jump_to_date")}
        </p>
        <input
          type="date"
          defaultValue={value.length === 10 ? value : undefined}
          onChange={handleChange}
          className="block w-full rounded-md border border-outline-variant/60 bg-surface-container-lowest px-3 py-1.5 text-sm text-on-surface focus:ring-2 focus:ring-primary/50 focus:outline-none"
        />
      </PopoverContent>
    </Popover>
  )
}
