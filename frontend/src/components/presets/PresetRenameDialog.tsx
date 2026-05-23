import { zodResolver } from "@hookform/resolvers/zod"
import { useForm } from "react-hook-form"
import { useTranslation } from "react-i18next"
import { z } from "zod"

import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form"
import { Input } from "@/components/ui/input"
import { presetNameSchema } from "@/lib/schemas/preset"

const renameSchema = z.object({ name: presetNameSchema })

type RenameValues = z.infer<typeof renameSchema>

interface PresetRenameDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  defaultName: string
  onSubmit: (name: string) => void
  pending?: boolean
}

export function PresetRenameDialog({
  open,
  onOpenChange,
  defaultName,
  onSubmit,
  pending,
}: PresetRenameDialogProps) {
  const { t } = useTranslation()
  const form = useForm<RenameValues>({
    resolver: zodResolver(renameSchema),
    defaultValues: { name: defaultName },
    values: { name: defaultName },
  })

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("preset.rename")}</DialogTitle>
        </DialogHeader>
        <Form {...form}>
          <form
            onSubmit={form.handleSubmit((v) => onSubmit(v.name.trim()))}
            className="space-y-4"
          >
            <FormField
              control={form.control}
              name="name"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("preset.name")}</FormLabel>
                  <FormControl>
                    <Input autoFocus {...field} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => onOpenChange(false)}
              >
                {t("common.cancel")}
              </Button>
              <Button type="submit" disabled={pending}>
                {t("common.save")}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  )
}
