import { createFileRoute } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"

import { PageHeader } from "@/components/editorial/PageHeader"
import { ProfileEditor } from "@/components/settings/ProfileEditor"
import { TimeSlotsEditor } from "@/components/settings/TimeSlotsEditor"

export const Route = createFileRoute("/settings/")({
  component: SettingsPage,
})

function SettingsPage() {
  const { t } = useTranslation()

  return (
    <div className="mx-auto max-w-5xl space-y-10 px-4 py-8 md:px-8 md:py-12">
      <PageHeader title={t("nav.settings")} />
      <div className="grid gap-10 lg:grid-cols-12">
        <section className="space-y-6 lg:col-span-7">
          <TimeSlotsEditor />
        </section>
        <section className="space-y-6 lg:col-span-5">
          <ProfileEditor />
        </section>
      </div>
    </div>
  )
}
