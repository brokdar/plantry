import { useMutation } from "@tanstack/react-query"

import { askHelp, type HelpInput } from "@/lib/api/help"

/** One help question; the sheet shows the error inline and offers a retry,
 *  so there is no toast. */
export function useAskHelp() {
  return useMutation({ mutationFn: (input: HelpInput) => askHelp(input) })
}
