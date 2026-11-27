import { Sparkles } from "lucide-react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import { useFillLibrary } from "@/lib/queries/chef"

/** "Fill my library" (COLD-3, Chef on): one tap starts ten recipes that fit
 *  the household; they arrive in the library on trial. */
export function FillLibrary() {
  const { t } = useTranslation()
  const fill = useFillLibrary()
  return (
    <div
      className="flex flex-wrap items-center gap-3"
      data-testid="fill-library"
    >
      <Button
        type="button"
        variant="outline"
        className="h-11 md:pointer-fine:h-9"
        disabled={fill.isPending || fill.isSuccess}
        onClick={() => fill.mutate()}
        data-testid="fill-library-button"
      >
        <Sparkles data-icon="inline-start" strokeWidth={1.5} aria-hidden />
        {t("recipes.fill.button")}
      </Button>
      <p className="text-small text-ink-muted">{t("recipes.fill.hint")}</p>
    </div>
  )
}
