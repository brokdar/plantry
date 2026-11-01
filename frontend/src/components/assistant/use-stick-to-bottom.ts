import { useCallback, useEffect, useRef } from "react"

/** Within this many px of the end counts as "at the bottom". */
export const BOTTOM_SLACK = 48

/** Whether a scroll position is at (or within the slack of) the bottom. */
export function isAtBottom(
  el: Pick<HTMLElement, "scrollHeight" | "scrollTop" | "clientHeight">
): boolean {
  return el.scrollHeight - el.scrollTop - el.clientHeight <= BOTTOM_SLACK
}

/**
 * Keeps a scroll container at its end as `content` changes, until the user
 * scrolls away from the bottom; scrolling back down resumes following.
 * `reset` (a new conversation) starts following again. Returns the scroll
 * handler for the container.
 */
export function useStickToBottom(
  ref: React.RefObject<HTMLElement | null>,
  content: unknown,
  reset: unknown
) {
  const following = useRef(true)

  const onScroll = useCallback(() => {
    if (ref.current) following.current = isAtBottom(ref.current)
  }, [ref])

  useEffect(() => {
    following.current = true
  }, [reset])

  useEffect(() => {
    const el = ref.current
    if (el && following.current) el.scrollTop = el.scrollHeight
  }, [ref, content, reset])

  return onScroll
}
