import { Bookmark } from "lucide-react"
import { useState } from "react"
import { useTranslation } from "react-i18next"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Separator } from "@/components/ui/separator"
import { Skeleton } from "@/components/ui/skeleton"
import type { Template } from "@/lib/api/templates"
import { useApplyTemplate } from "@/lib/queries/templates"
import { useTimeSlots } from "@/lib/queries/slots"
import { useTemplates } from "@/lib/queries/templates"
import { toast, toastError } from "@/lib/toast"

function todayISO(): string {
  const d = new Date()
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`
}

interface ApplyTemplateSectionProps {
  defaultSlotId?: string
  defaultDate?: string
}

export function ApplyTemplateSection({
  defaultSlotId,
  defaultDate,
}: ApplyTemplateSectionProps) {
  const { t } = useTranslation()
  const { data: templates, isLoading: tplLoading } = useTemplates()
  const { data: slotsData, isLoading: slotsLoading } = useTimeSlots(true)

  const [selectedTemplate, setSelectedTemplate] = useState<Template | null>(
    null
  )
  const [startDate, setStartDate] = useState(() => defaultDate ?? todayISO())
  const [slotId, setSlotId] = useState<string>(defaultSlotId ?? "")

  const applyMut = useApplyTemplate()

  const slots = slotsData?.items ?? []

  if (!tplLoading && (!templates || templates.length === 0)) return null

  function handlePickTemplate(tpl: Template) {
    setSelectedTemplate(tpl)
    // Pre-select first slot if none chosen yet
    if (!slotId && slots.length > 0) {
      setSlotId(String(slots[0]!.id))
    }
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!selectedTemplate || !slotId) return
    try {
      await applyMut.mutateAsync({
        templateId: selectedTemplate.id,
        input: { start_date: startDate, slot_id: Number(slotId) },
      })
      toast.success(t("template.applied"))
      setSelectedTemplate(null)
    } catch (err) {
      toastError(err, t)
    }
  }

  const isLoading = tplLoading || slotsLoading

  return (
    <div className="space-y-3" data-testid="apply-template-section">
      <div className="flex items-center gap-2">
        <Bookmark className="size-3.5 text-accent-foreground" aria-hidden />
        <p className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
          {t("template.apply_from")}
        </p>
      </div>
      {isLoading ? (
        <div className="space-y-2">
          <Skeleton className="h-10 w-full" />
          <Skeleton className="h-10 w-full" />
        </div>
      ) : (
        <>
          <ul className="space-y-1.5">
            {templates?.map((tpl) => (
              <li key={tpl.id}>
                <button
                  type="button"
                  onClick={() => handlePickTemplate(tpl)}
                  className={`group flex w-full items-center justify-between gap-3 rounded-md border bg-card px-3 py-2 text-left transition hover:border-primary/40 hover:bg-accent/20 ${
                    selectedTemplate?.id === tpl.id
                      ? "border-primary/60 bg-accent/30"
                      : "border-border"
                  }`}
                  data-testid={`apply-template-${tpl.id}`}
                >
                  <span className="truncate text-sm font-medium">
                    {tpl.name}
                  </span>
                  <Badge variant="secondary" className="shrink-0 text-xs">
                    {t("template.components_count", {
                      count: tpl.components.length,
                    })}
                  </Badge>
                </button>
              </li>
            ))}
          </ul>

          {selectedTemplate && (
            <form
              onSubmit={(e) => void handleSubmit(e)}
              className="space-y-3 rounded-md border border-border bg-muted/40 px-3 py-3"
              data-testid="apply-template-form"
            >
              <div className="space-y-1.5">
                <Label htmlFor="apply-start-date" className="text-xs">
                  {t("template.apply.start_on")}
                </Label>
                <Input
                  id="apply-start-date"
                  type="date"
                  value={startDate}
                  onChange={(e) => setStartDate(e.target.value)}
                  required
                  data-testid="apply-start-date"
                />
              </div>

              <div className="space-y-1.5">
                <Label htmlFor="apply-slot" className="text-xs">
                  {t("template.apply.slot")}
                </Label>
                <Select value={slotId} onValueChange={setSlotId} required>
                  <SelectTrigger
                    id="apply-slot"
                    data-testid="apply-slot-select"
                  >
                    <SelectValue placeholder={t("template.apply.slot")} />
                  </SelectTrigger>
                  <SelectContent>
                    {slots.map((s) => (
                      <SelectItem key={s.id} value={String(s.id)}>
                        {t(s.name_key, { defaultValue: s.name_key })}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>

              <div className="flex items-center gap-2">
                <Button
                  type="submit"
                  size="sm"
                  disabled={!slotId || applyMut.isPending}
                  data-testid="apply-template-submit"
                >
                  {t("template.apply.submit")}
                </Button>
                <Button
                  type="button"
                  size="sm"
                  variant="ghost"
                  onClick={() => setSelectedTemplate(null)}
                >
                  {t("common.cancel")}
                </Button>
              </div>
            </form>
          )}
        </>
      )}
      <Separator className="opacity-60" />
    </div>
  )
}
