import { createFileRoute, Link, useNavigate } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { ChevronLeft } from "lucide-react"
import { z } from "zod/v4"

import { PageHeader } from "@/components/editorial/PageHeader"
import { IngredientEditor } from "@/components/ingredients/IngredientEditor"

const searchSchema = z.object({
  debug: z.boolean().optional(),
})

export const Route = createFileRoute("/ingredients/new")({
  component: NewIngredientPage,
  validateSearch: (search) => searchSchema.parse(search),
})

function NewIngredientPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()

  return (
    <div className="mx-auto max-w-6xl space-y-8 px-4 pt-8 pb-32 md:px-8 md:pt-12 md:pb-16">
      <PageHeader
        eyebrow={t("ingredient.edit_eyebrow")}
        title={t("ingredient.new_title")}
        description={t("ingredient.new_subtitle")}
        breadcrumb={
          <Link
            to="/ingredients"
            className="inline-flex items-center gap-1 hover:text-on-surface"
          >
            <ChevronLeft className="size-3" aria-hidden />
            {t("ingredient.breadcrumb_back")}
          </Link>
        }
      />
      <IngredientEditor onSuccess={() => navigate({ to: "/ingredients" })} />
    </div>
  )
}
