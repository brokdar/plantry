import { Link } from "@tanstack/react-router"
import { ArrowLeftRight, Ban, Undo2, X } from "lucide-react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import type { TasteFood, TasteItem } from "@/lib/api/taste"
import { chefPlanning } from "@/lib/chef-settings"
import { usePlan } from "@/lib/queries/plan"
import { useSettingItems } from "@/lib/queries/settings"
import { useTaste, useTasteEdits } from "@/lib/queries/taste"
import { themeLabel, useWindowThemes } from "@/lib/queries/themes"

import { SettingsGroup } from "../controls"

/** Settings → Taste (TASTE-11): what the household told the app and what the
 *  app noticed, in plain sentences. Shown with or without a key and with the
 *  Chef off. */
export function TasteSection() {
  const { data: taste } = useTaste()
  if (!taste) {
    return (
      <div className="flex flex-col gap-4" aria-hidden>
        <Skeleton className="h-16 w-full" />
        <Skeleton className="h-16 w-full" />
      </div>
    )
  }
  const stated = (i: TasteItem) => i.source !== "learned"
  const notCuisine = (i: TasteItem) => i.level !== "cuisine"
  const love = taste.items.filter(
    (i) => stated(i) && notCuisine(i) && i.polarity === "love"
  )
  const rather = taste.items.filter(
    (i) => stated(i) && notCuisine(i) && i.polarity === "rather_not"
  )
  const leanings = taste.items.filter((i) => !stated(i) && notCuisine(i))
  const cuisines = taste.items.filter((i) => i.level === "cuisine")
  return (
    <div className="flex flex-col gap-8" data-testid="taste-page">
      <NeverGroup foods={taste.never} />
      <ItemGroup id="love" items={love} />
      <ItemGroup id="rather_not" items={rather} />
      <ItemGroup id="leanings" items={leanings} />
      <StaplesGroup staples={taste.staples} />
      <ItemGroup id="cuisines" items={cuisines} />
      <KitchenLink />
      <ThemeGroup />
      <RecentGroup />
    </div>
  )
}

function Empty({ id }: { id: string }) {
  const { t } = useTranslation()
  return (
    <p className="py-3 text-small text-ink-muted">
      {t(`settings_page.taste.${id}.empty`)}
    </p>
  )
}

function NeverGroup({ foods }: { foods: TasteFood[] }) {
  const { t } = useTranslation()
  const { removeNever } = useTasteEdits()
  return (
    <SettingsGroup
      title={t("settings_page.taste.never.title")}
      description={t("settings_page.taste.never.effect")}
    >
      {foods.length === 0 && <Empty id="never" />}
      <ul data-testid="taste-never">
        {foods.map((f) => (
          <li
            key={f.id}
            className="flex items-center justify-between gap-3 border-b border-line py-2 text-body text-ink last:border-b-0"
          >
            <span>{t("settings_page.taste.never.line", { name: f.name })}</span>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => removeNever.mutate(f.id)}
              aria-label={t("settings_page.taste.remove_label", {
                name: f.name,
              })}
              className="pointer-coarse:h-11"
            >
              <X data-icon="inline-start" strokeWidth={1.5} aria-hidden />
              {t("settings_page.taste.remove")}
            </Button>
          </li>
        ))}
      </ul>
    </SettingsGroup>
  )
}

/** What an item is about, in a few words ("Thai", "spicy dishes"). */
function useAbout() {
  const { t } = useTranslation()
  return (i: TasteItem): string => {
    const plain = i.key.replaceAll("_", " ")
    switch (i.level) {
      case "cuisine":
        return t(`attributes.cuisine.${i.key}`, { defaultValue: plain })
      case "technique":
        return t(`attributes.technique.${i.key}`, { defaultValue: plain })
      case "format":
        return t(`attributes.format.${i.key}`, { defaultValue: plain })
      case "flavour":
        return t(`settings_page.taste.flavour.${i.key}`, {
          defaultValue: plain,
        })
      default:
        return plain
    }
  }
}

function ItemGroup({ id, items }: { id: string; items: TasteItem[] }) {
  const { t } = useTranslation()
  return (
    <SettingsGroup
      title={t(`settings_page.taste.${id}.title`)}
      description={t(`settings_page.taste.${id}.effect`)}
    >
      {items.length === 0 && <Empty id={id} />}
      <ul data-testid={`taste-${id}`}>
        {items.map((i) => (
          <ItemRow key={i.id} item={i} group={id} />
        ))}
      </ul>
    </SettingsGroup>
  )
}

function ItemRow({ item, group }: { item: TasteItem; group: string }) {
  const { t } = useTranslation()
  const about = useAbout()(item)
  const { flip, remove, makeNever } = useTasteEdits()
  const flipped = item.polarity === "love" ? "rather_not" : "love"
  const sentence =
    group === "leanings"
      ? t(`settings_page.taste.sentence.leans_${item.polarity}`, {
          name: about,
        })
      : t(`settings_page.taste.sentence.${item.polarity}`, { name: about })
  const act = "pointer-coarse:h-11"
  return (
    <li
      data-testid="taste-item"
      className="flex flex-col gap-2 border-b border-line py-3 last:border-b-0 sm:flex-row sm:items-center sm:justify-between"
    >
      <span className="flex flex-col">
        <span className="text-body text-ink">{sentence}</span>
        <span className="text-small text-ink-muted" data-testid="taste-source">
          {t(`settings_page.taste.source.${item.source}`)}
        </span>
      </span>
      <span className="flex flex-wrap gap-2">
        <Button
          type="button"
          variant="outline"
          size="sm"
          className={act}
          onClick={() => flip.mutate({ id: item.id, polarity: flipped })}
        >
          <ArrowLeftRight
            data-icon="inline-start"
            strokeWidth={1.5}
            aria-hidden
          />
          {t(`settings_page.taste.flip_to.${flipped}`)}
        </Button>
        {item.level === "hero" && (
          <Button
            type="button"
            variant="outline"
            size="sm"
            className={act}
            onClick={() => makeNever.mutate(item.id)}
          >
            <Ban data-icon="inline-start" strokeWidth={1.5} aria-hidden />
            {t("settings_page.taste.make_never")}
          </Button>
        )}
        <Button
          type="button"
          variant="outline"
          size="sm"
          className={act}
          onClick={() => remove.mutate(item.id)}
          aria-label={t("settings_page.taste.remove_label", { name: about })}
        >
          <X data-icon="inline-start" strokeWidth={1.5} aria-hidden />
          {t("settings_page.taste.remove")}
        </Button>
      </span>
    </li>
  )
}

function StaplesGroup({ staples }: { staples: TasteFood[] }) {
  const { t } = useTranslation()
  return (
    <SettingsGroup
      title={t("settings_page.taste.staples.title")}
      description={t("settings_page.taste.staples.effect")}
    >
      {staples.length === 0 && <Empty id="staples" />}
      <ul data-testid="taste-staples">
        {staples.map((f) => (
          <li
            key={f.id}
            className="border-b border-line py-2 text-body text-ink last:border-b-0"
          >
            <Link to="/recipes/$id" params={{ id: String(f.id) }}>
              {f.name}
            </Link>
          </li>
        ))}
      </ul>
    </SettingsGroup>
  )
}

/** This week's theme (THEME-2): it ends with the window; the plan page
 *  sets it. Only with the Chef on and a provider, like the picker. */
function ThemeGroup() {
  const { t } = useTranslation()
  const on = chefPlanning(useSettingItems().data)
  const plan = usePlan()
  const start = plan.data?.window.start ?? ""
  const themes = useWindowThemes(start, on && start !== "").data?.themes
  if (!on || !themes) return null
  return (
    <SettingsGroup title={t("settings_page.taste.theme.title")}>
      <p className="py-3 text-body text-ink">
        {themes.length > 0
          ? t("settings_page.taste.theme.line", {
              names: themes.map((th) => themeLabel(th.value)).join(", "),
            })
          : t("settings_page.taste.theme.empty")}{" "}
        <Link to="/plan" className="text-accent underline">
          {t("settings_page.taste.theme.link")}
        </Link>
      </p>
    </SettingsGroup>
  )
}

function KitchenLink() {
  const { t } = useTranslation()
  return (
    <SettingsGroup title={t("settings_page.taste.kitchen.title")}>
      <p className="py-3 text-body text-ink">
        {t("settings_page.taste.kitchen.line")}{" "}
        <Link
          to="/settings/$section"
          params={{ section: "kitchen" }}
          className="text-accent underline"
        >
          {t("settings_page.taste.kitchen.link")}
        </Link>
      </p>
    </SettingsGroup>
  )
}

function RecentGroup() {
  const { t } = useTranslation()
  const { data } = useTaste()
  const { undo } = useTasteEdits()
  const about = useAbout()
  const recent = data?.recent ?? []
  return (
    <SettingsGroup
      title={t("settings_page.taste.recent.title")}
      description={t("settings_page.taste.recent.effect")}
    >
      {recent.length === 0 && <Empty id="recent" />}
      <ul data-testid="taste-recent">
        {recent.map((r) => (
          <li
            key={r.undo.first_event_id}
            className="flex items-center justify-between gap-3 border-b border-line py-2 text-body text-ink last:border-b-0"
          >
            <span>
              {t(`settings_page.taste.sentence.leans_${r.item.polarity}`, {
                name: about(r.item),
              })}
            </span>
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="pointer-coarse:h-11"
              disabled={undo.isPending}
              onClick={() => undo.mutate(r.undo)}
            >
              <Undo2 data-icon="inline-start" strokeWidth={1.5} aria-hidden />
              {t("common.undo")}
            </Button>
          </li>
        ))}
      </ul>
    </SettingsGroup>
  )
}
