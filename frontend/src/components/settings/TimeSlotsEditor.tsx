import { useState, useDeferredValue } from "react"
import { zodResolver } from "@hookform/resolvers/zod"
import { ChevronsUpDown, Search, Trash2 } from "lucide-react"
import { useForm, type Resolver } from "react-hook-form"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
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
import {
  SLOT_ICONS,
  SLOT_ICON_FALLBACK,
  SLOT_ICON_NAMES,
} from "@/lib/slot-icons"
import { slotLabel } from "@/lib/slot-label"

import { cn } from "@/lib/utils"

function SlotIcon({ name, className }: { name: string; className?: string }) {
  const Icon = SLOT_ICONS[name] ?? SLOT_ICON_FALLBACK
  return (
    <Icon className={cn("size-4", className)} strokeWidth={1.5} aria-hidden />
  )
}

function IconPicker({
  value,
  onChange,
}: {
  value: string
  onChange: (v: string) => void
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState("")
  const deferred = useDeferredValue(search)

  const icons = deferred
    ? SLOT_ICON_NAMES.filter((n) =>
        n.toLowerCase().includes(deferred.toLowerCase())
      )
    : SLOT_ICON_NAMES

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="outline"
          role="combobox"
          aria-expanded={open}
          className="w-full justify-between gap-2 pointer-coarse:h-11"
        >
          <span className="flex items-center gap-2 truncate">
            <SlotIcon name={value} />
            <span className="truncate text-small">
              {value || (
                <span className="text-ink-muted">
                  {t("slot.icon_pick_placeholder")}
                </span>
              )}
            </span>
          </span>
          <ChevronsUpDown
            className="size-4 shrink-0 text-ink-muted"
            strokeWidth={1.5}
            aria-hidden
          />
        </Button>
      </PopoverTrigger>
      <PopoverContent
        align="start"
        className="w-[var(--radix-popover-trigger-width)] gap-0 p-0"
      >
        <div className="flex items-center gap-2 border-b border-line px-3 py-2">
          <Search
            className="size-4 shrink-0 text-ink-muted"
            strokeWidth={1.5}
            aria-hidden
          />
          <Input
            autoFocus
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder={t("slot.icon_search_placeholder")}
            aria-label={t("slot.icon_search_placeholder")}
            className="border-0 bg-transparent px-0 shadow-none"
          />
        </div>
        {!deferred && (
          <p className="px-3 pt-2 pb-1 text-caption text-ink-muted">
            {t("slot.icon_suggested")}
          </p>
        )}
        <div className="max-h-64 overflow-y-auto p-2">
          {icons.length === 0 ? (
            <p className="py-6 text-center text-small text-ink-muted">
              {t("slot.icon_none_found")}
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
                    "flex min-h-11 flex-col items-center justify-center gap-1 rounded-cell p-2 text-center text-ink hover:bg-panel-sunken",
                    value === name && "bg-accent text-on-accent hover:bg-accent"
                  )}
                >
                  <SlotIcon name={name} className="size-5" />
                  <span className="w-full truncate text-caption">{name}</span>
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
      form.setError("root", { message: t(key) })
    }
  }

  async function handleDelete(s: TimeSlot) {
    if (!window.confirm(t("slot.delete_confirm_body"))) return
    await deleteMut.mutateAsync(s.id).catch(() => {})
  }

  const items = slotsQuery.data?.items ?? []

  return (
    <div className="flex flex-col gap-6">
      <section className="flex flex-col gap-4 rounded-lg border border-line bg-panel p-4 md:p-6">
        <h4 className="font-sans text-body font-semibold text-ink">
          {t("slot.add_slot")}
        </h4>
        <Form {...form}>
          <form
            noValidate
            onSubmit={form.handleSubmit(onSubmit)}
            // Two columns until the content column is wide enough for all
            // four controls; minmax(0, …) lets the icon picker shrink.
            className="grid gap-4 sm:grid-cols-2 sm:items-end xl:grid-cols-[minmax(0,2fr)_minmax(0,1fr)_minmax(0,1fr)_auto]"
          >
            {form.formState.errors.root && (
              <p role="alert" className="col-span-full text-small text-danger">
                {form.formState.errors.root.message}
              </p>
            )}
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
                      className="pointer-coarse:h-11"
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
                    <IconPicker value={field.value} onChange={field.onChange} />
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
                      className="tabular-nums pointer-coarse:h-11"
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <Button
              type="submit"
              disabled={createMut.isPending}
              className="justify-self-start pointer-coarse:h-11"
            >
              {t("common.save")}
            </Button>
          </form>
        </Form>
      </section>

      <section className="rounded-lg border border-line bg-panel">
        {items.length === 0 ? (
          <p className="p-6 text-body text-ink-muted">{t("slot.no_slots")}</p>
        ) : (
          <ul className="divide-y divide-line">
            {items.map((s) => (
              <li
                key={s.id}
                className="flex items-center gap-4 p-4"
                data-testid={`slot-row-${s.id}`}
              >
                <SlotIcon name={s.icon} className="text-ink-muted" />
                <div className="flex-1">
                  <p className="text-body font-semibold text-ink">
                    {slotLabel(t, s.name_key)}
                  </p>
                  <p className="text-small text-ink-muted">
                    {t("slot.row_detail", {
                      key: s.name_key,
                      order: s.sort_order,
                    })}
                  </p>
                </div>
                <Button
                  variant="ghost"
                  size="icon"
                  className="pointer-coarse:size-11"
                  aria-label={t("common.delete")}
                  onClick={() => handleDelete(s)}
                >
                  <Trash2 strokeWidth={1.5} aria-hidden />
                </Button>
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  )
}
