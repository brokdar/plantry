import { Heart, ThumbsDown, ThumbsUp } from "lucide-react"
import { useTranslation } from "react-i18next"

import { cn } from "@/lib/utils"

interface SlotActionsProps {
  favorite: boolean
  loved: boolean
  disliked: boolean
  onFavorite: () => void
  onLove: () => void
  onDislike: () => void
}

export function SlotActions({
  favorite,
  loved,
  disliked,
  onFavorite,
  onLove,
  onDislike,
}: SlotActionsProps) {
  const { t } = useTranslation()
  return (
    <div className="absolute top-2 right-2 flex gap-1 opacity-0 transition-opacity group-focus-within:opacity-100 group-hover:opacity-100">
      <ActionBtn
        label={t("planner.slot.action.favorite")}
        testid="slot-action-favorite"
        onClick={onFavorite}
        active={favorite}
        activeClass="text-[#c44a4a]"
      >
        <Heart
          className="h-3.5 w-3.5"
          fill={favorite ? "currentColor" : "none"}
          aria-hidden
        />
      </ActionBtn>
      <ActionBtn
        label={t("planner.slot.action.love")}
        testid="slot-action-love"
        onClick={onLove}
        active={loved}
        activeClass="text-primary"
      >
        <ThumbsUp
          className="h-3.5 w-3.5"
          fill={loved ? "currentColor" : "none"}
          aria-hidden
        />
      </ActionBtn>
      <ActionBtn
        label={t("planner.slot.action.dislike")}
        testid="slot-action-dislike"
        onClick={onDislike}
        active={disliked}
        activeClass="text-destructive"
      >
        <ThumbsDown
          className="h-3.5 w-3.5"
          fill={disliked ? "currentColor" : "none"}
          aria-hidden
        />
      </ActionBtn>
    </div>
  )
}

interface ActionBtnProps {
  label: string
  testid: string
  onClick: () => void
  active: boolean
  activeClass: string
  children: React.ReactNode
}

function ActionBtn({
  label,
  testid,
  onClick,
  active,
  activeClass,
  children,
}: ActionBtnProps) {
  return (
    <button
      type="button"
      aria-label={label}
      aria-pressed={active}
      data-testid={testid}
      onClick={(e) => {
        e.stopPropagation()
        onClick()
      }}
      className={cn(
        "grid size-7 place-items-center rounded-full border border-black/5 bg-white/90 text-on-surface-variant shadow-sm backdrop-blur transition-colors hover:text-on-surface",
        active && activeClass
      )}
    >
      {children}
    </button>
  )
}
