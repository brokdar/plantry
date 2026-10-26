import { createRouter } from "@tanstack/react-router"

import { routeTree } from "./routeTree.gen"

export const router = createRouter({
  routeTree,
  defaultPreload: "intent",
  scrollRestoration: true,
})

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router
  }
}

declare module "@tanstack/history" {
  interface HistoryState {
    /** AppShell moves focus to `<main>` on arrival (e.g. from the setup
     *  wizard, which renders without the shell). */
    focusMain?: boolean
  }
}
