import {
  FoodPlaceholder,
  type FoodPlaceholderCategory,
} from "@/components/editorial/FoodPlaceholder"
import { imageURL } from "@/lib/image-url"

interface SlotHeroProps {
  imagePath: string | null | undefined
  role: string | null
  roleLabel: string
}

export function SlotHero({ imagePath, role, roleLabel }: SlotHeroProps) {
  return (
    <div className="relative h-24 overflow-hidden">
      {imagePath ? (
        <img
          src={imageURL(imagePath)}
          alt=""
          className="h-full w-full object-cover"
        />
      ) : (
        <FoodPlaceholder
          category={(role ?? "main") as FoodPlaceholderCategory}
          size="md"
          rounded="none"
          className="h-full w-full"
        />
      )}
      {imagePath && (
        <div
          className="absolute inset-0 bg-gradient-to-b from-transparent from-40% to-black/40"
          aria-hidden
        />
      )}
      <span className="absolute bottom-2 left-2 font-heading text-[9.5px] font-bold tracking-[0.16em] text-white uppercase drop-shadow-[0_1px_2px_rgba(0,0,0,0.45)]">
        {roleLabel}
      </span>
    </div>
  )
}
