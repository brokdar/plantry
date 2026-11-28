import { useMutation } from "@tanstack/react-query"

import {
  askHelp,
  setHelpVerdict,
  type HelpInput,
  type HelpVerdict,
} from "@/lib/api/help"

/** One help question; the sheet shows the error inline and offers a retry,
 *  so there is no toast. */
export function useAskHelp() {
  return useMutation({ mutationFn: (input: HelpInput) => askHelp(input) })
}

/** The verdict is quiet: a failed save shows nothing and the button stays
 *  pressable. */
export function useHelpVerdict() {
  return useMutation({
    mutationFn: (v: { runId: number; verdict: HelpVerdict }) =>
      setHelpVerdict(v.runId, v.verdict),
  })
}
