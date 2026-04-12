import { createFileRoute } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"

export const Route = createFileRoute("/")({
  component: HomePage,
})

function HomePage() {
  const { t } = useTranslation()

  return (
    <section className="flex flex-col gap-3">
      <h1 className="text-2xl font-semibold tracking-tight">
        {t("home.title")}
      </h1>
      <p className="text-muted-foreground">{t("home.subtitle")}</p>
    </section>
  )
}
