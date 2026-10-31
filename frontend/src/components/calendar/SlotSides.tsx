import { useTranslation } from "react-i18next"

import {
  FoodPlaceholder,
  type FoodPlaceholderCategory,
} from "@/components/editorial/FoodPlaceholder"
import { imageURL } from "@/lib/image-url"

export interface SlotSideItem {
  imagePath: string | null | undefined
  role: string | null
  name: string
}

interface SlotSidesProps {
  items: SlotSideItem[]
  max?: number
}

/**
 * Avatar-group preview of a plate's side components. Replaces SlotChips:
 * tiny truncated text labels lost the legibility race once cells got dense.
 * Up to `max` thumbnails render in a row with a slight overlap; anything
 * beyond is summarised with a "+N" badge. Names are joined into a single
 * aria-label so screen readers still hear the full list.
 */
export function SlotSides({ items, max = 3 }: SlotSidesProps) {
  const { t } = useTranslation()
  if (items.length === 0) return null

  const visible = items.slice(0, max)
  const overflow = items.length - visible.length
  const groupLabel = items.map((i) => i.name).join(", ")

  return (
    <div
      className="flex items-center"
      role="list"
      aria-label={t("planner.slot.sides_label", { names: groupLabel })}
    >
      {visible.map((item, i) => (
        <Thumb
          key={`${item.name}-${i}`}
          item={item}
          // Stack the thumbs by lifting each subsequent one slightly to the
          // left of the previous edge — gives the avatar-group illusion
          // without resorting to negative margin gymnastics on the wrapper.
          className={i === 0 ? "" : "-ml-1.5"}
        />
      ))}
      {overflow > 0 && (
        <span
          className="-ml-1.5 grid size-[18px] place-items-center rounded-full border border-outline-variant/60 bg-surface-container-low font-heading text-[9px] font-bold tracking-tight text-on-surface-variant tabular-nums"
          aria-hidden
        >
          +{overflow}
        </span>
      )}
    </div>
  )
}

function Thumb({ item, className }: { item: SlotSideItem; className: string }) {
  return (
    <span
      role="listitem"
      title={item.name}
      aria-label={item.name}
      className={`relative grid size-[18px] place-items-center overflow-hidden rounded-full border border-surface-container-lowest bg-surface-container-low shadow-[0_0_0_0.5px_rgba(25,28,28,0.08)] ${className}`}
    >
      {item.imagePath ? (
        <img
          src={imageURL(item.imagePath)}
          alt=""
          className="h-full w-full object-cover"
        />
      ) : (
        <FoodPlaceholder
          category={(item.role ?? "default") as FoodPlaceholderCategory}
          size="sm"
          rounded="none"
          className="h-full w-full"
        />
      )}
    </span>
  )
}
