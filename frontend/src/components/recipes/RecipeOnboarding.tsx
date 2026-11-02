import { useEffect, useId } from "react"
import { Link } from "@tanstack/react-router"
import { Compass, Download, PencilLine, type LucideIcon } from "lucide-react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import { toast } from "@/lib/toast"
import { cn } from "@/lib/utils"

/** Plan decision D18. "shown" once the guided state rendered; "done" once the
 *  milestone toast fired, so it never fires twice. */
export const RECIPE_GUIDE_KEY = "plantry.recipeGuideShown"

function readFlag(): string | null {
  try {
    return localStorage.getItem(RECIPE_GUIDE_KEY)
  } catch {
    return null
  }
}

function writeFlag(value: string) {
  try {
    localStorage.setItem(RECIPE_GUIDE_KEY, value)
  } catch {
    // Storage blocked: the toast is a nicety, the guide still works.
  }
}

type RecipeOnboardingProps = {
  count: number
  target: number
  /** Discovery is available (an AI provider is set): adds the third card. */
  discover: boolean
}

/**
 * Guided Recipes state (UX §5.8): a progress row and the ways to add recipes
 * (import, discover when available, create), shown while the library is below
 * the planning target. Renders nothing once the target is reached, and fires
 * the one-time milestone toast then.
 */
export function RecipeOnboarding({
  count,
  target,
  discover,
}: RecipeOnboardingProps) {
  const { t } = useTranslation()
  const progressId = useId()
  const guided = count < target

  useEffect(() => {
    const flag = readFlag()
    if (guided) {
      if (flag !== "done") writeFlag("shown")
      return
    }
    if (flag === "shown") {
      writeFlag("done")
      toast.success(t("component.guide.milestone", { count: target }))
    }
  }, [guided, target, t])

  if (!guided) return null

  const percent = Math.round((Math.min(count, target) / target) * 100)

  return (
    <section
      aria-label={t("component.guide.label")}
      className="flex flex-col gap-4"
      data-testid="recipe-onboarding"
    >
      <div className="flex flex-col gap-2">
        <p id={progressId} className="text-body text-ink">
          {t("component.guide.progress", { count, target })}
        </p>
        <div
          role="progressbar"
          aria-labelledby={progressId}
          aria-valuemin={0}
          aria-valuemax={target}
          aria-valuenow={count}
          aria-valuetext={t("component.guide.progress_value", {
            count,
            target,
          })}
          className="h-2 w-full max-w-md overflow-hidden rounded-full border border-ink-muted bg-panel"
        >
          <div
            className="h-full rounded-full bg-accent transition-[width] duration-200 ease-out"
            style={{ width: `${percent}%` }}
          />
        </div>
      </div>

      <div
        className={cn(
          "grid grid-cols-1 gap-4 sm:grid-cols-2",
          discover && "lg:grid-cols-3"
        )}
      >
        <OptionCard
          icon={Download}
          title={t("component.guide.import_title")}
          body={t("component.guide.import_body")}
          action={
            <Button asChild className="h-11 px-4">
              <Link
                to="/recipes"
                search={{ add: "url" }}
                data-testid="onboarding-import"
              >
                {t("component.guide.import_action")}
              </Link>
            </Button>
          }
        />
        {discover && (
          <OptionCard
            icon={Compass}
            title={t("component.guide.discover_title")}
            body={t("component.guide.discover_body")}
            action={
              <Button asChild variant="outline" className="h-11 px-4">
                <Link
                  to="/recipes"
                  search={{ add: "discover" }}
                  data-testid="onboarding-discover"
                >
                  {t("component.guide.discover_action")}
                </Link>
              </Button>
            }
          />
        )}
        <OptionCard
          icon={PencilLine}
          title={t("component.guide.create_title")}
          body={t("component.guide.create_body")}
          action={
            <Button asChild variant="outline" className="h-11 px-4">
              <Link to="/recipes/new" data-testid="onboarding-create">
                {t("component.guide.create_action")}
              </Link>
            </Button>
          }
        />
      </div>
    </section>
  )
}

function OptionCard({
  icon: Icon,
  title,
  body,
  action,
}: {
  icon: LucideIcon
  title: string
  body: string
  action: React.ReactNode
}) {
  return (
    <div className="flex flex-col items-start gap-3 rounded-lg border border-line bg-panel p-4">
      <Icon className="size-5 text-accent" strokeWidth={1.5} aria-hidden />
      <div className="flex flex-col gap-1">
        <h2 className="text-h3 font-semibold text-ink">{title}</h2>
        <p className="max-w-[60ch] text-small text-ink-muted">{body}</p>
      </div>
      <div className="mt-auto pt-1">{action}</div>
    </div>
  )
}
