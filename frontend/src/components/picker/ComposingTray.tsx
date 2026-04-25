import { Minus, Plus, X } from "lucide-react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import {
  FoodPlaceholder,
  type FoodPlaceholderCategory,
} from "@/components/editorial/FoodPlaceholder"
import type { Food } from "@/lib/api/foods"
import { imageURL } from "@/lib/image-url"
import { cn } from "@/lib/utils"

export interface TrayItem {
  component: Food
  portions: number
}

interface ComposingTrayProps {
  items: TrayItem[]
  onPortionChange: (componentId: number, portions: number) => void
  onRemove: (componentId: number) => void
  onSave: () => void
  onCancel: () => void
  saving?: boolean
}

export function ComposingTray({
  items,
  onPortionChange,
  onRemove,
  onSave,
  onCancel,
  saving,
}: ComposingTrayProps) {
  const { t } = useTranslation()
  return (
    <aside className="editorial-shadow sticky top-6 flex flex-col gap-4 self-start rounded-[22px] border border-outline-variant/50 bg-surface-container-lowest p-6">
      <header className="flex items-baseline justify-between">
        <h3 className="font-heading text-[17px] font-bold">
          {t("picker.tray.title")}
        </h3>
        <span className="font-heading text-[12px] text-on-surface-variant">
          {t("picker.tray.count", {
            count: items.length,
            defaultValue: "{{count}} components",
          })}
        </span>
      </header>
      <p className="text-[12.5px] text-on-surface-variant italic">
        {t("picker.tray.empty")}
      </p>
      {items.length > 0 ? (
        <ul className="flex flex-col gap-2" data-testid="picker-tray-list">
          {items.map((it, idx) => {
            const hero = idx === 0
            return (
              <li
                key={it.component.id}
                data-testid={`tray-item-${it.component.id}`}
                className={cn(
                  "flex items-center gap-2.5 rounded-[10px] bg-surface-container-low p-2",
                  hero && "bg-primary-fixed/60"
                )}
              >
                <div
                  className={cn(
                    "aspect-[4/3] h-[38px] shrink-0 overflow-hidden rounded-lg",
                    hero && "outline outline-2 outline-primary"
                  )}
                >
                  {it.component.image_path ? (
                    <img
                      src={imageURL(it.component.image_path)}
                      alt=""
                      className="h-full w-full object-cover"
                    />
                  ) : (
                    <FoodPlaceholder
                      category={it.component.role as FoodPlaceholderCategory}
                      size="sm"
                      rounded="none"
                      className="h-full w-full"
                    />
                  )}
                </div>
                <div className="min-w-0 flex-1">
                  <p className="font-heading text-[10.5px] font-semibold tracking-[0.08em] text-on-surface-variant uppercase">
                    {hero
                      ? `${t("picker.tray.hero_prefix", { defaultValue: "Hero" })} · ${t(`planner.slot.role.${it.component.role}`, { defaultValue: it.component.role })}`
                      : t(`planner.slot.role.${it.component.role}`, {
                          defaultValue: it.component.role,
                        })}
                  </p>
                  <p className="truncate font-heading text-[12.5px] font-semibold">
                    {it.component.name}
                  </p>
                </div>
                <div
                  className="flex items-center gap-1.5"
                  data-testid={`tray-portion-${it.component.id}`}
                >
                  <button
                    type="button"
                    aria-label={t("common.decrease", {
                      defaultValue: "Decrease",
                    })}
                    onClick={() =>
                      onPortionChange(
                        it.component.id,
                        Math.max(0.5, Math.round((it.portions - 0.5) * 2) / 2)
                      )
                    }
                    className="grid size-6 place-items-center rounded-full border border-outline-variant bg-white font-heading text-[13px] font-bold hover:bg-surface-container-high"
                  >
                    <Minus className="h-3 w-3" aria-hidden />
                  </button>
                  <span className="min-w-[24px] text-center font-heading text-[12px] font-bold">
                    {it.portions}×
                  </span>
                  <button
                    type="button"
                    aria-label={t("common.increase", {
                      defaultValue: "Increase",
                    })}
                    onClick={() =>
                      onPortionChange(
                        it.component.id,
                        Math.round((it.portions + 0.5) * 2) / 2
                      )
                    }
                    className="grid size-6 place-items-center rounded-full border border-outline-variant bg-white font-heading text-[13px] font-bold hover:bg-surface-container-high"
                  >
                    <Plus className="h-3 w-3" aria-hidden />
                  </button>
                </div>
                <button
                  type="button"
                  aria-label={t("common.remove", { defaultValue: "Remove" })}
                  onClick={() => onRemove(it.component.id)}
                  className="grid size-6 place-items-center rounded-full text-on-surface-variant hover:bg-surface-container-high"
                >
                  <X className="h-3 w-3" aria-hidden />
                </button>
              </li>
            )
          })}
        </ul>
      ) : null}

      <div className="flex flex-col gap-2 pt-2">
        <Button
          onClick={onSave}
          disabled={items.length === 0 || saving}
          data-testid="tray-save"
          className="h-11 w-full justify-center"
        >
          {t("picker.tray.save")}
        </Button>
        <Button
          variant="outline"
          onClick={onCancel}
          data-testid="tray-cancel"
          className="h-11 w-full justify-center"
        >
          {t("picker.tray.cancel")}
        </Button>
      </div>
    </aside>
  )
}
