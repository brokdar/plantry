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

interface RoutineRenameDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  defaultName: string
  onSubmit: (name: string) => void
  pending?: boolean
}

export function RoutineRenameDialog({
  open,
  onOpenChange,
  defaultName,
  onSubmit,
  pending,
}: RoutineRenameDialogProps) {
  const { t } = useTranslation()
  const form = useForm<RenameValues>({
    resolver: zodResolver(renameSchema),
    defaultValues: { name: defaultName },
    values: { name: defaultName },
  })

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="bg-panel text-ink">
        <DialogHeader>
          <DialogTitle className="text-h3">
            {t("routines.rename_title")}
          </DialogTitle>
        </DialogHeader>
        <Form {...form}>
          <form
            onSubmit={form.handleSubmit((v) => onSubmit(v.name.trim()))}
            className="flex flex-col gap-4"
          >
            <FormField
              control={form.control}
              name="name"
              render={({ field }) => (
                <FormItem>
                  <FormLabel className="text-small text-ink-muted">
                    {t("routines.name")}
                  </FormLabel>
                  <FormControl>
                    <Input
                      autoFocus
                      className="pointer-coarse:h-11"
                      {...field}
                    />
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
