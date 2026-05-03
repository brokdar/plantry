import {
  FoodPlaceholder,
  type FoodPlaceholderCategory,
} from "@/components/editorial/FoodPlaceholder"
import { imageURL } from "@/lib/image-url"
import { cn } from "@/lib/utils"

export interface SlotHeroComponent {
  imagePath: string | null | undefined
  role: string | null
  name: string
}

interface SlotHeroProps {
  components: SlotHeroComponent[]
  /** Editorial eyebrow over the hero image. Pass `null`/`undefined` for leaf
   *  foods (no role taxonomy) — the eyebrow stays hidden rather than
   *  surfacing a generic "INGREDIENT" word that scans as noise. */
  heroRoleLabel: string | null | undefined
  /** Cell height variant. `tall` adds breathing room for collages on the
   *  desktop planned cell; `compact` keeps the original 96 px height for
   *  surfaces that haven't opted into the taller cell yet. */
  variant?: "tall" | "compact"
}

const HERO_HEIGHT: Record<NonNullable<SlotHeroProps["variant"]>, string> = {
  tall: "h-28",
  compact: "h-24",
}

export function SlotHero({
  components,
  heroRoleLabel,
  variant = "tall",
}: SlotHeroProps) {
  const count = components.length
  const hero = components[0]

  return (
    <div className={cn("relative overflow-hidden", HERO_HEIGHT[variant])}>
      {count <= 1 ? (
        <Tile component={hero} fallbackRole="main" />
      ) : count === 2 ? (
        <TwoUpCollage components={components} />
      ) : (
        <ThreeUpCollage components={components} />
      )}

      {hero && hero.imagePath && (
        <div
          className="pointer-events-none absolute inset-0 bg-gradient-to-b from-transparent from-40% to-black/40"
          aria-hidden
        />
      )}

      {heroRoleLabel && (
        <span className="absolute bottom-2 left-2 font-heading text-[9.5px] font-bold tracking-[0.16em] text-white uppercase drop-shadow-[0_1px_2px_rgba(0,0,0,0.45)]">
          {heroRoleLabel}
        </span>
      )}

      {count > 1 && (
        <span
          className="absolute top-1.5 left-1.5 grid min-w-[20px] place-items-center rounded-full bg-on-surface/80 px-1.5 py-px font-heading text-[10px] font-bold tracking-[0.04em] text-on-primary tabular-nums shadow-sm backdrop-blur-[2px]"
          aria-hidden
        >
          {count}
        </span>
      )}
    </div>
  )
}

function TwoUpCollage({ components }: { components: SlotHeroComponent[] }) {
  return (
    <div className="grid h-full grid-cols-2 gap-px bg-outline-variant/30">
      <Tile component={components[0]} fallbackRole="main" />
      <Tile component={components[1]} fallbackRole="side_veg" />
    </div>
  )
}

function ThreeUpCollage({ components }: { components: SlotHeroComponent[] }) {
  // 3+: hero takes the left 60 %, two stacked thumbs on the right 40 %.
  // For 4+ we render the first 3 and let the count badge tell the user
  // there's more — the collage gets too noisy past 3 panes.
  return (
    <div className="grid h-full grid-cols-[3fr_2fr] gap-px bg-outline-variant/30">
      <Tile component={components[0]} fallbackRole="main" />
      <div className="grid grid-rows-2 gap-px">
        <Tile component={components[1]} fallbackRole="side_veg" />
        <Tile component={components[2]} fallbackRole="side_starch" />
      </div>
    </div>
  )
}

function Tile({
  component,
  fallbackRole,
}: {
  component: SlotHeroComponent | undefined
  fallbackRole: FoodPlaceholderCategory
}) {
  const role = (component?.role ?? fallbackRole) as FoodPlaceholderCategory
  if (component?.imagePath) {
    return (
      <img
        src={imageURL(component.imagePath)}
        alt=""
        className="h-full w-full object-cover"
      />
    )
  }
  return (
    <FoodPlaceholder
      category={role}
      size="md"
      rounded="none"
      className="h-full w-full"
    />
  )
}
