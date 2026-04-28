import { useNavigate, useSearch } from "@tanstack/react-router"
import {
  CalendarDays,
  Cpu,
  Database,
  Palette,
  Salad,
  UtensilsCrossed,
  type LucideIcon,
} from "lucide-react"
import { useTranslation } from "react-i18next"

import { PageHeader } from "@/components/editorial/PageHeader"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { TooltipProvider } from "@/components/ui/tooltip"

import { AITab } from "./tabs/AITab"
import { GeneralTab } from "./tabs/GeneralTab"
import { PlanTab } from "./tabs/PlanTab"
import { MealSlotsTab } from "./tabs/MealSlotsTab"
import { NutritionTab } from "./tabs/NutritionTab"
import { SystemTab } from "./tabs/SystemTab"

export type SettingsTab =
  | "general"
  | "plan"
  | "ai"
  | "nutrition"
  | "meal_slots"
  | "system"

const TABS: Array<{ value: SettingsTab; Icon: LucideIcon }> = [
  { value: "general", Icon: Palette },
  { value: "plan", Icon: CalendarDays },
  { value: "ai", Icon: Cpu },
  { value: "nutrition", Icon: Salad },
  { value: "meal_slots", Icon: UtensilsCrossed },
  { value: "system", Icon: Database },
]

export function SettingsShell() {
  const { t } = useTranslation()
  const search = useSearch({ from: "/settings/" }) as { tab?: SettingsTab }
  const navigate = useNavigate({ from: "/settings/" })
  const active: SettingsTab = search.tab ?? "general"

  return (
    <TooltipProvider delayDuration={200}>
      <div className="mx-auto max-w-6xl space-y-10 px-4 py-8 md:px-8 md:py-12">
        <PageHeader
          eyebrow={t("settings_page.eyebrow")}
          title={t("settings_page.title")}
          description={t("settings_page.subtitle")}
        />
        <Tabs
          value={active}
          onValueChange={(next) =>
            navigate({ search: { tab: next as SettingsTab } })
          }
          orientation="vertical"
          className="md:flex-row"
        >
          <TabsList
            variant="line"
            className="min-w-48 items-stretch border-r border-outline/50 pr-4"
          >
            {TABS.map(({ value, Icon }) => (
              <TabsTrigger
                key={value}
                value={value}
                className="justify-start gap-3 px-3 py-2 text-left text-sm data-active:text-primary group-data-[variant=line]/tabs-list:data-active:after:bg-primary"
                data-testid={`settings-tab-${value}`}
              >
                <Icon className="size-4 text-on-surface-variant" aria-hidden />
                {t(`settings_page.tabs.${value}`)}
              </TabsTrigger>
            ))}
          </TabsList>
          <div className="min-w-0 flex-1 pl-2 md:pl-8">
            <TabsContent value="general">
              <GeneralTab />
            </TabsContent>
            <TabsContent value="plan">
              <PlanTab />
            </TabsContent>
            <TabsContent value="ai">
              <AITab />
            </TabsContent>
            <TabsContent value="nutrition">
              <NutritionTab />
            </TabsContent>
            <TabsContent value="meal_slots">
              <MealSlotsTab />
            </TabsContent>
            <TabsContent value="system">
              <SystemTab />
            </TabsContent>
          </div>
        </Tabs>
      </div>
    </TooltipProvider>
  )
}
