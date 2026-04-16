import { zodResolver } from "@hookform/resolvers/zod"
import * as Lucide from "lucide-react"
import { Trash2 } from "lucide-react"
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
import { ApiError } from "@/lib/api/client"
import type { TimeSlot } from "@/lib/api/slots"
import {
  useCreateTimeSlot,
  useDeleteTimeSlot,
  useTimeSlots,
} from "@/lib/queries/slots"
import { slotSchema, type SlotFormValues } from "@/lib/schemas/slot"

function SlotIcon({ name }: { name: string }) {
  const Icon = (
    Lucide as unknown as Record<string, Lucide.LucideIcon | undefined>
  )[name]
  if (!Icon) return <Lucide.HelpCircle className="h-4 w-4" aria-hidden />
  return <Icon className="h-4 w-4" aria-hidden />
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
      window.alert(
        err instanceof ApiError ? t(err.messageKey) : t("error.server")
      )
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
                      <Input
                        {...field}
                        placeholder={t("slot.icon_placeholder")}
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
                    <p className="font-medium">
                      {t(s.name_key, { defaultValue: s.name_key })}
                    </p>
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
