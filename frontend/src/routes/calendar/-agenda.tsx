import type { InfiniteData } from "@tanstack/react-query"

import { AgendaList } from "@/components/calendar/AgendaList"
import type { Food } from "@/lib/api/foods"
import type { Plate } from "@/lib/api/plates"
import type { TimeSlot } from "@/lib/api/slots"
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
  foodsById: Map<number, Food>
  slots: TimeSlot[]
}

export function AgendaView({
  data,
  hasNextPage,
  isFetchingNextPage,
  fetchNextPage,
  search,
  weekStartsOn,
  foodsById,
  slots,
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
      foodsById={foodsById}
      slots={slots}
    />
  )
}
