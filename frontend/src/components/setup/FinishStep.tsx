import { useNavigate } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import { useCompleteSetup } from "@/lib/queries/setup"

/** "You're set up." Add recipes marks setup done, then opens Recipes. */
export function FinishStep({
  headingRef,
}: {
  headingRef: React.Ref<HTMLHeadingElement>
}) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const complete = useCompleteSetup()

  function addRecipes() {
    complete.mutate(undefined, {
      onSuccess: () =>
        void navigate({ to: "/components", state: { focusMain: true } }),
    })
  }

  return (
    <div className="flex flex-col gap-6" data-testid="setup-finish">
      <div className="flex flex-col gap-2">
        <h1 ref={headingRef} tabIndex={-1} className="outline-none">
          {t("setup.finish.title")}
        </h1>
        <p className="text-body text-ink-muted">{t("setup.finish.body")}</p>
      </div>
      <Button
        type="button"
        onClick={addRecipes}
        disabled={complete.isPending}
        className="self-start pointer-coarse:h-11"
      >
        {t("setup.finish.add_recipes")}
      </Button>
    </div>
  )
}
