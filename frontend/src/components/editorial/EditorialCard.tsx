import { cn } from "@/lib/utils"

type CommonProps = {
  children: React.ReactNode
  className?: string
  interactive?: boolean
  testId?: string
}

type StaticProps = CommonProps & {
  as?: "div" | "article"
  href?: never
  onClick?: never
}

type LinkProps = CommonProps & {
  as?: never
  href: string
  onClick?: never
}

type ButtonProps = CommonProps & {
  as?: never
  href?: never
  onClick: () => void
}

export type EditorialCardProps = StaticProps | LinkProps | ButtonProps

export function EditorialCard(props: EditorialCardProps) {
  const { children, className, interactive, testId } = props

  const commonClass = cn(
    "editorial-shadow group flex flex-col overflow-hidden rounded-xl bg-surface-container-lowest text-left transition-all duration-300",
    interactive && "cursor-pointer hover:-translate-y-1",
    className
  )

  if ("href" in props && props.href !== undefined) {
    return (
      <a href={props.href} className={commonClass} data-testid={testId}>
        {children}
      </a>
    )
  }

  if ("onClick" in props && props.onClick !== undefined) {
    return (
      <button
        type="button"
        onClick={props.onClick}
        className={commonClass}
        data-testid={testId}
      >
        {children}
      </button>
    )
  }

  const Comp = (props as StaticProps).as ?? "article"
  return (
    <Comp className={commonClass} data-testid={testId}>
      {children}
    </Comp>
  )
}

EditorialCard.Image = function EditorialCardImage({
  src,
  alt,
  className,
  children,
}: {
  src?: string
  alt?: string
  className?: string
  children?: React.ReactNode
}) {
  return (
    <div
      className={cn(
        "pointer-events-none relative m-2 aspect-[4/3] overflow-hidden rounded-xl bg-surface-container-high",
        className
      )}
    >
      {src ? (
        <img
          src={src}
          alt={alt ?? ""}
          className="h-full w-full object-cover transition-transform duration-500 group-hover:scale-105"
        />
      ) : (
        <div className="flex h-full w-full items-center justify-center text-on-surface-variant/40">
          {children}
        </div>
      )}
      {children && src && <div className="absolute inset-0">{children}</div>}
    </div>
  )
}

EditorialCard.Body = function EditorialCardBody({
  children,
  className,
}: {
  children: React.ReactNode
  className?: string
}) {
  return (
    <div className={cn("pointer-events-none px-6 pt-2 pb-6", className)}>
      {children}
    </div>
  )
}

EditorialCard.Title = function EditorialCardTitle({
  children,
  className,
}: {
  children: React.ReactNode
  className?: string
}) {
  return (
    <h3
      className={cn(
        "font-heading text-xl leading-tight font-bold text-on-surface",
        className
      )}
    >
      {children}
    </h3>
  )
}

EditorialCard.Meta = function EditorialCardMeta({
  children,
  className,
}: {
  children: React.ReactNode
  className?: string
}) {
  return (
    <div
      className={cn(
        "flex items-center gap-4 text-xs font-medium tracking-wider text-on-surface-variant uppercase",
        className
      )}
    >
      {children}
    </div>
  )
}

EditorialCard.ImageOverlay = function EditorialCardImageOverlay({
  position = "bottom-left",
  className,
  children,
}: {
  position?: "top-left" | "top-right" | "bottom-left" | "bottom-right"
  className?: string
  children: React.ReactNode
}) {
  const positionClass = {
    "top-left": "top-3 left-3",
    "top-right": "top-3 right-3",
    "bottom-left": "bottom-3 left-3",
    "bottom-right": "bottom-3 right-3",
  }[position]
  return (
    <div
      className={cn(
        "pointer-events-none absolute z-20 flex gap-1.5",
        positionClass,
        className
      )}
    >
      {children}
    </div>
  )
}

EditorialCard.Footer = function EditorialCardFooter({
  children,
  className,
}: {
  children: React.ReactNode
  className?: string
}) {
  return (
    <div
      className={cn(
        "border-t border-outline-variant/10 px-6 py-4 text-xs text-on-surface-variant",
        className
      )}
    >
      {children}
    </div>
  )
}
