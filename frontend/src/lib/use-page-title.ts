import { useRouterState } from "@tanstack/react-router"
import { useEffect } from "react"
import { useTranslation } from "react-i18next"

/** Page title key per top-level section; other paths keep the bare name. */
const SECTION_TITLES: Record<string, string> = {
  today: "nav.today",
  plan: "nav.plan",
  recipes: "nav.recipes",
  shop: "nav.shop",
  settings: "nav.settings",
}

/** Keeps `document.title` at "Today · Plantry" and so on, so tabs, history
 *  and screen readers can tell the views apart (WCAG 2.4.2). */
export function usePageTitle() {
  const { t } = useTranslation()
  const section = useRouterState({
    select: (s) => s.location.pathname.split("/")[1] ?? "",
  })
  const titleKey = SECTION_TITLES[section]
  const title = titleKey ? `${t(titleKey)} · ${t("nav.brand")}` : t("nav.brand")
  useEffect(() => {
    document.title = title
  }, [title])
}
