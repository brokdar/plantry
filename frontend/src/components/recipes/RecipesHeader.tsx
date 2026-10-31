import { Link } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"

import { cn } from "@/lib/utils"

const TAB_CLASS =
  "inline-flex min-h-11 items-center border-b-2 px-1 text-body transition-colors duration-150 ease-out md:pointer-fine:min-h-9"

/**
 * The Recipes page title, its two tabs (Recipes, Ingredients; UX §5.8) and
 * the tab's primary action at the top right. The router marks the current
 * tab with `aria-current`.
 */
export function RecipesHeader({
  tab,
  action,
}: {
  tab: "recipes" | "ingredients"
  action: React.ReactNode
}) {
  const { t } = useTranslation()
  const tabClass = (active: boolean) =>
    cn(
      TAB_CLASS,
      active
        ? "border-accent font-semibold text-ink"
        : "border-transparent text-ink-muted hover:text-ink"
    )

  return (
    <header className="flex flex-col gap-3">
      <div className="flex items-center justify-between gap-3">
        <h1 className="text-h1 text-ink">{t("recipes.title")}</h1>
        {action}
      </div>
      <nav
        aria-label={t("recipes.tabs_label")}
        className="flex gap-6 border-b border-line"
      >
        <Link
          to="/recipes"
          activeOptions={{ exact: true, includeSearch: false }}
          className={tabClass(tab === "recipes")}
          data-testid="recipes-tab-recipes"
        >
          {t("recipes.tab_recipes")}
        </Link>
        <Link
          to="/recipes/ingredients"
          className={tabClass(tab === "ingredients")}
          data-testid="recipes-tab-ingredients"
        >
          {t("recipes.tab_ingredients")}
        </Link>
      </nav>
    </header>
  )
}
