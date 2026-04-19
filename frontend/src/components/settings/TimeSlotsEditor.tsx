import { useState, useDeferredValue } from "react"
import { zodResolver } from "@hookform/resolvers/zod"
import * as Lucide from "lucide-react"
import { ChevronsUpDown, Search, Trash2 } from "lucide-react"
import { useForm, type Resolver } from "react-hook-form"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form"
import { Input } from "@/components/ui/input"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"
import { ApiError } from "@/lib/api/client"
import type { TimeSlot } from "@/lib/api/slots"
import {
  useCreateTimeSlot,
  useDeleteTimeSlot,
  useTimeSlots,
} from "@/lib/queries/slots"
import { slotSchema, type SlotFormValues } from "@/lib/schemas/slot"
import { slotLabel } from "@/lib/slot-label"
import { toastError } from "@/lib/toast"
import { cn } from "@/lib/utils"

const RECOMMENDED_ICONS = [
  "Coffee",
  "Sun",
  "Sunrise",
  "Sunset",
  "Moon",
  "UtensilsCrossed",
  "Pizza",
  "Apple",
  "Egg",
  "Fish",
  "Carrot",
  "Cookie",
  "Milk",
  "Wheat",
  "Leaf",
  "Soup",
  "Sandwich",
  "Cherry",
  "Heart",
  "Star",
  "Clock",
  "Timer",
  "Salad",
  "FlameKindling",
]

const ALL_ICON_NAMES: string[] = Object.keys(Lucide as Record<string, unknown>)
  .filter(
    (k) =>
      /^[A-Z]/.test(k) &&
      typeof (Lucide as Record<string, unknown>)[k] === "function"
  )
  .sort()

function SlotIcon({ name, className }: { name: string; className?: string }) {
  const Icon = (
    Lucide as unknown as Record<string, Lucide.LucideIcon | undefined>
  )[name]
  if (!Icon)
    return (
      <Lucide.HelpCircle className={cn("h-4 w-4", className)} aria-hidden />
    )
  return <Icon className={cn("h-4 w-4", className)} aria-hidden />
}

function IconPicker({
  value,
  onChange,
}: {
  value: string
  onChange: (v: string) => void
}) {
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState("")
  const deferred = useDeferredValue(search)

  const icons = deferred
    ? ALL_ICON_NAMES.filter((n) =>
        n.toLowerCase().includes(deferred.toLowerCase())
      ).slice(0, 100)
    : RECOMMENDED_ICONS

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="outline"
          role="combobox"
          aria-expanded={open}
          className="w-full justify-between gap-2"
        >
          <span className="flex items-center gap-2 truncate">
            <SlotIcon name={value} />
            <span className="truncate text-sm">
              {value || (
                <span className="text-muted-foreground">Pick an icon</span>
              )}
            </span>
          </span>
          <ChevronsUpDown className="size-4 shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent
        align="start"
        className="w-[var(--radix-popover-trigger-width)] gap-0 p-0"
      >
        <div className="flex items-center gap-2 border-b border-border px-3 py-2">
          <Search
            className="size-4 shrink-0 text-muted-foreground"
            aria-hidden
          />
          <Input
            autoFocus
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search icons…"
            className="border-0 bg-transparent px-0 shadow-none focus-visible:ring-0"
          />
        </div>
        {!deferred && (
          <p className="px-3 pt-2 pb-1 text-[10px] font-semibold tracking-widest text-muted-foreground uppercase">
            Suggested
          </p>
        )}
        <div className="max-h-64 overflow-y-auto p-2">
          {icons.length === 0 ? (
            <p className="py-6 text-center text-xs text-muted-foreground">
              No icons found
            </p>
          ) : (
            <div className="grid grid-cols-5 gap-1">
              {icons.map((name) => (
                <button
                  key={name}
                  type="button"
                  title={name}
                  onClick={() => {
                    onChange(name)
                    setSearch("")
                    setOpen(false)
                  }}
                  className={cn(
                    "flex flex-col items-center gap-1 rounded-md p-2 text-center hover:bg-accent focus:bg-accent focus:outline-none",
                    value === name && "bg-accent ring-1 ring-ring"
                  )}
                >
                  <SlotIcon name={name} className="h-5 w-5" />
                  <span className="w-full truncate text-[9px] text-muted-foreground">
                    {name}
                  </span>
                </button>
              ))}
            </div>
          )}
        </div>
      </PopoverContent>
    </Popover>
  )
}

export function TimeSlotsEditor() {
  const { t } = useTranslation()
  const slotsQuery = useTimeSlots(false)
  const createMut = useCreateTimeSlot()
  const deleteMut = useDeleteTimeSlot()

  const form = useForm<SlotFormValues>({
    resolver: zodResolver(slotSchema) as Resolver<SlotFormValues>,
    defaultValues: { name_key: "", icon: "", sort_order: 0, active: true },
  })

  async function onSubmit(values: SlotFormValues) {
    try {
      await createMut.mutateAsync(values)
      form.reset({
        name_key: "",
        icon: "",
        sort_order: (slotsQuery.data?.items.length ?? 0) + 1,
        active: true,
      })
    } catch (err) {
      const key = err instanceof ApiError ? err.messageKey : "error.server"
      form.setError("name_key", { message: key })
    }
  }

  async function handleDelete(s: TimeSlot) {
    if (!window.confirm(t("slot.delete_confirm_body"))) return
    try {
      await deleteMut.mutateAsync(s.id)
    } catch (err) {
      toastError(err, t)
    }
  }

  const items = slotsQuery.data?.items ?? []

  return (
    <div className="flex flex-col gap-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">
          {t("slot.title")}
        </h1>
        <p className="text-sm text-muted-foreground">{t("slot.description")}</p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>{t("slot.add_slot")}</CardTitle>
        </CardHeader>
        <CardContent>
          <Form {...form}>
            <form
              noValidate
              onSubmit={form.handleSubmit(onSubmit)}
              className="grid gap-4 sm:grid-cols-[2fr_1fr_1fr_auto] sm:items-end"
            >
              <FormField
                control={form.control}
                name="name_key"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t("slot.name_key_label")}</FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        placeholder={t("slot.name_key_placeholder")}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="icon"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t("slot.icon_label")}</FormLabel>
                    <FormControl>
                      <IconPicker
                        value={field.value}
                        onChange={field.onChange}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="sort_order"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t("slot.sort_order_label")}</FormLabel>
                    <FormControl>
                      <Input
                        type="number"
                        {...field}
                        onChange={(e) => field.onChange(Number(e.target.value))}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <Button type="submit" disabled={createMut.isPending}>
                {t("common.save")}
              </Button>
            </form>
          </Form>
        </CardContent>
      </Card>

      <Card>
        <CardContent className="p-0">
          {items.length === 0 ? (
            <p className="p-6 text-sm text-muted-foreground">
              {t("slot.no_slots")}
            </p>
          ) : (
            <ul className="divide-y divide-border">
              {items.map((s) => (
                <li
                  key={s.id}
                  className="flex items-center gap-4 p-4"
                  data-testid={`slot-row-${s.id}`}
                >
                  <SlotIcon name={s.icon} />
                  <div className="flex-1">
                    <p className="font-medium">{slotLabel(t, s.name_key)}</p>
                    <p className="text-xs text-muted-foreground">
                      {s.name_key} · {t("slot.sort_order_label")}:{" "}
                      {s.sort_order}
                    </p>
                  </div>
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label={t("common.delete")}
                    onClick={() => handleDelete(s)}
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
