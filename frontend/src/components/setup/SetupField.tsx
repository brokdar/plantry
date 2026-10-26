/** One labelled control in a wizard step: label, an optional effect line,
 *  then the control. Stacked, since the column is narrow on every size. */
export function SetupField({
  label,
  description,
  htmlFor,
  children,
}: {
  label: string
  description?: React.ReactNode
  /** id of the control, so the label names it. */
  htmlFor?: string
  children: React.ReactNode
}) {
  return (
    <div className="flex flex-col gap-3 border-b border-line py-5 last:border-b-0">
      <div className="flex flex-col gap-1">
        {htmlFor ? (
          <label htmlFor={htmlFor} className="text-body font-semibold text-ink">
            {label}
          </label>
        ) : (
          <p className="text-body font-semibold text-ink">{label}</p>
        )}
        {description && (
          <p className="text-small text-ink-muted">{description}</p>
        )}
      </div>
      {children}
    </div>
  )
}
