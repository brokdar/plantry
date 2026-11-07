/**
 * Moves focus to the start of the page content: the page's h1, else `<main>`.
 * Used where the control that held focus goes away (an answered question, a
 * closed sheet) or the view changes under it, so focus never falls to
 * `<body>`.
 */
export function focusPageStart() {
  const main = document.getElementById("main")
  const target = main?.querySelector<HTMLElement>("h1") ?? main
  if (!target) return
  if (!target.hasAttribute("tabindex")) target.setAttribute("tabindex", "-1")
  target.focus({ preventScroll: true })
}
