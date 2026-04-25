import { zodResolver } from "@hookform/resolvers/zod"
import { X } from "lucide-react"
import { useRef, useState } from "react"
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
import { Textarea } from "@/components/ui/textarea"
import { ApiError } from "@/lib/api/client"
import { useProfile, useUpdateProfile } from "@/lib/queries/profile"
import { profileSchema, type ProfileFormValues } from "@/lib/schemas/profile"

const PRESETS = {
  cut: { kcal_target: 1800, protein_pct: 35, fat_pct: 30, carbs_pct: 35 },
  maintain: { kcal_target: 2200, protein_pct: 30, fat_pct: 30, carbs_pct: 40 },
  bulk: { kcal_target: 2800, protein_pct: 25, fat_pct: 25, carbs_pct: 50 },
} as const

export function ProfileEditor() {
  const { t } = useTranslation()
  const { data: profile } = useProfile()
  const updateMut = useUpdateProfile()
  const [saveSuccess, setSaveSuccess] = useState(false)
  const [restriction, setRestriction] = useState("")
  const restrictionRef = useRef<HTMLInputElement>(null)

  const form = useForm<ProfileFormValues>({
    resolver: zodResolver(profileSchema) as Resolver<ProfileFormValues>,
    defaultValues: {
      kcal_target: null,
      protein_pct: null,
      fat_pct: null,
      carbs_pct: null,
      dietary_restrictions: [],
      system_prompt: null,
      locale: "en",
    },
    values: profile
      ? {
          kcal_target: profile.kcal_target,
          protein_pct: profile.protein_pct,
          fat_pct: profile.fat_pct,
          carbs_pct: profile.carbs_pct,
          dietary_restrictions: profile.dietary_restrictions ?? [],
          system_prompt: profile.system_prompt,
          locale: profile.locale ?? "en",
        }
      : undefined,
    resetOptions: { keepDirtyValues: true },
  })

  function applyPreset(preset: keyof typeof PRESETS) {
    const p = PRESETS[preset]
    form.setValue("kcal_target", p.kcal_target, { shouldDirty: true })
    form.setValue("protein_pct", p.protein_pct, { shouldDirty: true })
    form.setValue("fat_pct", p.fat_pct, { shouldDirty: true })
    form.setValue("carbs_pct", p.carbs_pct, { shouldDirty: true })
  }

  const proteinPct = form.watch("protein_pct") ?? 0
  const fatPct = form.watch("fat_pct") ?? 0
  const carbsPct = form.watch("carbs_pct") ?? 0
  const macroSum = (proteinPct + fatPct + carbsPct).toFixed(1)

  const restrictions = form.watch("dietary_restrictions") ?? []

  function addRestriction() {
    const trimmed = restriction.trim()
    if (!trimmed || restrictions.includes(trimmed)) return
    form.setValue("dietary_restrictions", [...restrictions, trimmed], {
      shouldDirty: true,
    })
    setRestriction("")
    restrictionRef.current?.focus()
  }

  function removeRestriction(tag: string) {
    form.setValue(
      "dietary_restrictions",
      restrictions.filter((r) => r !== tag),
      { shouldDirty: true }
    )
  }

  async function onSubmit(values: ProfileFormValues) {
    try {
      await updateMut.mutateAsync({
        kcal_target: values.kcal_target ?? null,
        protein_pct: values.protein_pct ?? null,
        fat_pct: values.fat_pct ?? null,
        carbs_pct: values.carbs_pct ?? null,
        dietary_restrictions: values.dietary_restrictions ?? [],
        system_prompt: values.system_prompt ?? null,
        locale: values.locale,
      })
      setSaveSuccess(true)
      setTimeout(() => setSaveSuccess(false), 3000)
    } catch (err) {
      const key = err instanceof ApiError ? err.messageKey : "error.server"
      form.setError("root", { message: t(key) })
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("profile.title")}</CardTitle>
        <p className="text-sm text-muted-foreground">
          {t("profile.description")}
        </p>
      </CardHeader>
      <CardContent>
        <Form {...form}>
          <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-6">
            {/* Goal presets */}
            <div className="space-y-2">
              <p className="text-sm font-medium">{t("profile.goal_preset")}</p>
              <div className="flex gap-2">
                {(["cut", "maintain", "bulk"] as const).map((preset) => (
                  <Button
                    key={preset}
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() => applyPreset(preset)}
                  >
                    {t(`profile.preset_${preset}`)}
                  </Button>
                ))}
              </div>
            </div>

            {/* Calorie target */}
            <FormField
              control={form.control}
              name="kcal_target"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("profile.kcal_target")}</FormLabel>
                  <FormControl>
                    <Input
                      type="number"
                      min={1}
                      placeholder={t("profile.kcal_target_placeholder")}
                      value={field.value ?? ""}
                      onChange={(e) =>
                        field.onChange(
                          e.target.value === "" ? null : Number(e.target.value)
                        )
                      }
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            {/* Macro percentages */}
            <div className="space-y-3">
              <div className="flex items-center justify-between">
                <p className="text-sm font-medium">
                  {t("profile.macros_label")}
                </p>
                <span className="text-xs text-muted-foreground">
                  {t("profile.macro_sum", { sum: macroSum })}
                </span>
              </div>
              <div className="grid grid-cols-3 items-start gap-3">
                {(
                  [
                    ["protein_pct", "profile.protein_pct"],
                    ["fat_pct", "profile.fat_pct"],
                    ["carbs_pct", "profile.carbs_pct"],
                  ] as const
                ).map(([name, labelKey]) => (
                  <FormField
                    key={name}
                    control={form.control}
                    name={name}
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t(labelKey)}</FormLabel>
                        <FormControl>
                          <Input
                            type="number"
                            min={0}
                            max={100}
                            step={1}
                            placeholder="0"
                            value={field.value ?? ""}
                            onChange={(e) =>
                              field.onChange(
                                e.target.value === ""
                                  ? null
                                  : Number(e.target.value)
                              )
                            }
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                ))}
              </div>
            </div>

            {/* Dietary restrictions chip editor */}
            <div className="space-y-2">
              <p className="text-sm font-medium">
                {t("profile.dietary_restrictions")}
              </p>
              <div className="flex flex-wrap gap-2">
                {restrictions.map((tag) => (
                  <span
                    key={tag}
                    className="flex items-center gap-1 rounded-full bg-secondary px-3 py-1 text-sm"
                  >
                    {tag}
                    <button
                      type="button"
                      onClick={() => removeRestriction(tag)}
                      className="ml-1 rounded-full hover:text-destructive"
                      aria-label={t("profile.remove_restriction", { tag })}
                    >
                      <X className="h-3 w-3" />
                    </button>
                  </span>
                ))}
              </div>
              <div className="flex gap-2">
                <Input
                  ref={restrictionRef}
                  value={restriction}
                  onChange={(e) => setRestriction(e.target.value)}
                  placeholder={t("profile.restriction_placeholder")}
                  onKeyDown={(e) => {
                    if (e.key === "Enter") {
                      e.preventDefault()
                      addRestriction()
                    }
                  }}
                  className="max-w-xs"
                />
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={addRestriction}
                >
                  {t("profile.add_restriction")}
                </Button>
              </div>
            </div>

            {/* System prompt */}
            <FormField
              control={form.control}
              name="system_prompt"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("profile.system_prompt")}</FormLabel>
                  <FormControl>
                    <Textarea
                      placeholder={t("profile.system_prompt_placeholder")}
                      rows={3}
                      value={field.value ?? ""}
                      onChange={(e) =>
                        field.onChange(
                          e.target.value === "" ? null : e.target.value
                        )
                      }
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            {form.formState.errors.root && (
              <p className="text-sm text-destructive">
                {form.formState.errors.root.message}
              </p>
            )}
            <div className="flex items-center gap-3">
              <Button type="submit" disabled={updateMut.isPending}>
                {t("profile.save")}
              </Button>
              {saveSuccess && (
                <span className="text-sm text-primary">
                  {t("profile.save_success")}
                </span>
              )}
            </div>
          </form>
        </Form>
      </CardContent>
    </Card>
  )
}
