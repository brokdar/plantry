import type { InfiniteData } from "@tanstack/react-query"

import { AgendaList } from "@/components/calendar/AgendaList"
import type { Plate } from "@/lib/api/plates"
import { flattenPlatesPages } from "@/lib/queries/plates"

interface PlatesPage {
  plates: Plate[]
  from: string
  to: string
}

interface AgendaViewProps {
  data: InfiniteData<PlatesPage> | undefined
  hasNextPage: boolean
  isFetchingNextPage: boolean
  fetchNextPage: () => void
  search: string
  weekStartsOn: 0 | 1 | 6
  showCopyButton?: boolean
}

export function AgendaView({
  data,
  hasNextPage,
  isFetchingNextPage,
  fetchNextPage,
  search,
  weekStartsOn,
  showCopyButton,
}: AgendaViewProps) {
  const plates = flattenPlatesPages(data)

  return (
    <AgendaList
      plates={plates}
      hasNextPage={hasNextPage}
      isFetchingNextPage={isFetchingNextPage}
      fetchNextPage={fetchNextPage}
      search={search}
      weekStartsOn={weekStartsOn}
      showCopyButton={showCopyButton}
    />
  )
}
