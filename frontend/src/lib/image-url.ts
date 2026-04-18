export function imageURL(
  path: string | null | undefined,
  version?: string | number | null
): string | undefined {
  if (!path) return undefined
  const suffix = version ? `?v=${encodeURIComponent(String(version))}` : ""
  return `/images/${path}${suffix}`
}
