// The e2e recipe site for discovery (Phase 5 D20): the pages and images the
// fake research client points at. Started by playwright.config.ts as a
// third webServer on 9100 + E2E_LANE. broken.html is deliberately absent
// (a 404). {{SITE}} in a page becomes the request's host.
const port = Number(process.env.E2E_SITE_PORT ?? 9100)
const root = import.meta.dir

const files: Record<string, { file: string; type: string }> = {
  "/fesenjan.html": { file: "fesenjan.html", type: "text/html; charset=utf-8" },
  "/pasta.html": { file: "pasta.html", type: "text/html; charset=utf-8" },
  "/img/fesenjan.jpg": { file: "img/fesenjan.jpg", type: "image/jpeg" },
  "/img/pasta.jpg": { file: "img/pasta.jpg", type: "image/jpeg" },
}

Bun.serve({
  port,
  async fetch(req) {
    const hit = files[new URL(req.url).pathname]
    if (!hit) return new Response("not found", { status: 404 })
    const file = Bun.file(`${root}/${hit.file}`)
    // Pages name their image by absolute URL (the importer does not resolve
    // relative ones), so {{SITE}} becomes the host the request came to.
    const body = hit.type.startsWith("text/html")
      ? (await file.text()).replaceAll(
          "{{SITE}}",
          `http://${req.headers.get("host")}`
        )
      : file
    return new Response(body, { headers: { "content-type": hit.type } })
  },
})
