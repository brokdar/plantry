# Rating Rubric

Each section is rated on a 1–5 scale. The anchors below define what each number means so ratings are consistent across pages and reviewers. When in doubt between two numbers, pick the lower one and explain in the findings — conservative ratings push real issues to the top of the fix list.

| Score | Meaning |
|-------|---------|
| **5** | Production-ready polish. Nothing to change. Exemplary of how this area should work across the app. |
| **4** | Solid. Minor nits only; no user impact. Fine to ship. |
| **3** | Usable, but gaps are visible. Something a careful reviewer would flag; no one is blocked. |
| **2** | Noticeable problems. A real user would stumble, misunderstand, or feel uncertain. Fix before next release. |
| **1** | Broken or absent. Blocks use, hides critical info, or violates core app expectations. Ship-blocker. |

## Sections

Rate each of the following. Every rating needs a one-line justification and at least one piece of evidence (file path + line, or screenshot reference).

### 1. Component Architecture

- Is the component tree sensibly decomposed? Are children single-purpose?
- Do props communicate intent, or are there booleans stacking up (`isEditing`, `isNew`, `isDisabled`, `isReadOnly`)? Prop proliferation → composition refactor.
- Are hooks used correctly (deps arrays, no state-in-effect, no ref-in-render)?
- Is shared logic extracted into `@/lib` or a custom hook, or is it copy-pasted?
- Does the file follow the project's conventions for that area (`frontend/src/components/<area>/`)?

### 2. Layout & Visual Design

- Does the page match the Botanical Atelier system (editorial typography, OKLch tokens, Manrope, `PageHeader` eyebrow pattern)?
- Is visual hierarchy clear — the user's eye lands on the primary action first?
- Spacing scale consistent with sibling pages? No magic pixel values?
- Does the layout breathe, or does content fight for space?

### 3. Accessibility

- Semantic HTML — headings in order, buttons are `<button>`, forms have `<label htmlFor>`.
- Keyboard: Tab order matches visual order; every interactive element is reachable and has a visible focus ring.
- ARIA where semantics fall short: `aria-label` for icon buttons, `aria-describedby` for form errors, `role` only where needed.
- Contrast meets WCAG AA against the OKLch surface tokens.
- `prefers-reduced-motion` respected if animations are present.

### 4. Interaction Clarity & Affordances

- Can a first-time user tell what is clickable without hovering?
- Primary CTA is visually dominant vs secondary actions?
- Icon-only buttons have tooltips or visible labels?
- Hover state on every interactive element (button, card, row, icon)?
- Cursor changes on hover (`cursor-pointer` on real buttons, default on disabled)?
- Disabled state is obvious — reduced opacity *and* no hover response?

### 5. User Workflow

- Does the page tell the user what to do next at every step?
- Is there a moment where the user could be aimless — "I filled everything in, now what?"
- Is the happy path obvious? Are edge branches (empty, loading, error) equally guided?
- Navigation entry and exit — can the user get here and leave without thinking?
- Breadcrumbs / back buttons where the user could lose orientation?

### 6. Notifications & Feedback

- Toast on success with a specific message ("Ingredient saved", not "Success").
- Optimistic UI where the operation is safe; rollback on error.
- Pending states — button shows a spinner and is disabled during submit?
- Long operations (import, AI generation) show progress, not just a spinner?

### 7. Error Handling

- Form validation errors appear inline, next to the field, with clear wording.
- Network errors have a recovery path — retry, not just "Something went wrong".
- 404 / 403 / 5xx surfaces a meaningful page, not a blank screen.
- Empty states distinguish "no data yet" from "error loading" from "filtered away".

### 8. Scrolling & Responsive

- Mobile viewport (375×812): no horizontal scroll, touch targets ≥ 44px, content legible without zoom.
- Scroll areas have momentum and don't lock the parent scroll.
- Sticky elements (headers, action bars) don't cover content on small screens.
- Long lists virtualize or paginate — they don't stall scroll at 1000 items.

### 9. Search & Filter

- Only rate if the page has a list or catalog. Otherwise mark **N/A**.
- Search is fast (debounced or deferred) and doesn't refetch on every keystroke.
- Filters are visible, not hidden in a kebab menu, unless the page is filter-heavy enough to warrant a drawer.
- Clear-all is one click. Empty result state tells the user which filter to loosen.
- Current filter/search state is visible in the URL so the page is shareable.

### 10. Animations & Micro-interactions

Animations are the single biggest difference between a page that feels *built* and a page that feels *polished*. A page rated 5 here feels alive at every state change; a page rated 1 feels like static HTML with cursor interactions. Rate across these criteria — a page missing *any* of them deserves no higher than a 3.

**Presence (do animations exist at all?)**

- Modals and dialogs animate in/out (not pop-in, not fade only).
- Toasts slide in, pause, and slide out on dismiss.
- Dropdowns, popovers, sheets animate open with purposeful motion.
- Form section collapse/expand is smooth, not jump-cut.

**State transitions**

- Disabled → enabled: no animation required, but the cursor/shadow change must be instant — no lingering disabled styling.
- Loading → loaded: skeleton shimmer or progressive reveal preferred over a raw spinner that just disappears.
- Empty → filled (e.g., a list gets its first item): the first item enters, doesn't just appear.
- Error appearing: shake on invalid field, color tween, or subtle scale — not a hard red switch.
- Success appearing: a brief, confident motion on the confirmation element.

**Hover and press micro-interactions**

- Primary buttons: background tween, optional slight scale or shadow shift on press.
- Cards: border/shadow lift on hover when clickable.
- List rows: row-level hover treatment so the user tracks which row they're in.
- Icons inside buttons: subtle color or opacity shift, not jarring.

**Craft**

- Durations: 150–250ms for most UI motion; up to 400ms for larger transitions (modal open, page transition). Anything longer than 400ms is usually wrong.
- Easing: natural curves (`ease-out` for entries, `ease-in` for exits, a custom cubic-bezier for playful surfaces). **Linear on anything user-initiated is a finding.**
- No layout-shift animations — never animate `height` on a list; use max-height with overflow or react-resizable-panels.
- No duplicate animations fighting each other (e.g., a toast animating in while the button behind it animates out).

**Professionalism signals**

- Does any interaction feel "reluctant" — a pause before the dropdown opens, a beat of nothing after click? That's a ship-blocker for premium feel.
- Does motion reinforce information (this became that) or decorate (this just moved because)? Reinforcement = 4-5; decoration = 2-3.
- Does `prefers-reduced-motion` gracefully swap transitions for instant state changes? The animation reduction should *still look intentional*, not like the page fell back to HTML 3.2.

**Rating guide**

- **5**: every criterion met, motion is a first-class citizen, interactions feel confident. The kind of page that is referenced when other teams ask "how do you want this to feel?"
- **4**: most criteria met, one or two rough edges (a dialog that pops instead of animating, a toast that disappears abruptly).
- **3**: functional motion (spinners, hover colors) but nothing that makes the page feel alive. Modals pop. Lists jump. Forms feel static.
- **2**: noticeable missing motion — users click and aren't sure what happened. Some interactions feel like reloads.
- **1**: the page feels like an unstyled form. Cursor is the only affordance.

### 11. Information Completeness

- Does the user have every piece of information they need to finish the task on this page, without opening a second tab or leaving the page?
- Are units, dates, and currency formatted the way a human reads them?
- Context that helps the user decide (e.g., "this will replace 3 ingredients") is present before the action, not after.
- Secondary info (timestamps, IDs) is available for recovery but not in the user's face.

### 12. Internationalization

- Every user-visible string goes through `t(…)`. Search the component for quoted strings that aren't keys.
- Every key appears in both `frontend/src/lib/i18n/en.json` and `de.json`. A missing key in either locale is a P0.
- Plurals use `{{count}}` with the right ICU form, not ad-hoc `+ "s"`.
- Dates/numbers use locale-aware formatters, not hardcoded `toLocaleString("en-US")`.

### 13. Consistency

- Design tokens from `frontend/src/index.css`, not arbitrary hex values.
- Button variants, input styles, spacing match sibling pages. A new variant invented just for this page is a finding unless the use case genuinely demanded it.
- Shadcn primitives used where they exist — no parallel implementation of `Dialog`, `Tooltip`, etc.

### 14. Performance

- Only render what's visible. Heavy sub-components behind collapse/tab states don't mount eagerly.
- `useDeferredValue` over debounced effects for derived UI.
- Memoization used where it matters (expensive render path with stable inputs), not sprinkled by default.
- TanStack Query keys stable — no new object identity per render.

### 15. Navigation & Routing

Navigation is the connective tissue between pages. A user who clicks a button and lands somewhere unexpected loses trust; a user who clicks "Save" and watches the page reload their input is lost. Rate on where clicks take the user, whether the browser's back/forward respects context, and whether the URL accurately reflects state.

**Click destinations**

- Every clickable element lands where the user expects. A card linking to `/ingredients/$id/edit` when the user expected a preview modal is a finding unless the intent is explicitly signaled.
- Primary actions navigate on **confirmed success** (server returned 2xx), not optimistically before the server acknowledges — otherwise a failed save silently steals the user's form.
- Secondary/cancel actions go back where the user came from, not to the root. If the user navigates in from `/ingredients?q=milk`, Cancel should land back on `/ingredients?q=milk`, not `/ingredients`.
- External links open in a new tab with `rel="noopener noreferrer"`. Internal links never do.

**Scroll and focus on navigation**

- Landing on a new page scrolls to top (or to the hash target); it does not inherit the previous page's scroll position.
- Focus moves to the new page's primary heading, main content region, or first form field — screen-reader users should hear "you've arrived" instead of being stranded mid-DOM.
- Returning via the back button restores scroll position *and* form state where possible (TanStack Router does this when keys are stable).
- In-page anchor jumps animate-scroll rather than hard-jump; anchors inside collapsed sections expand the section before scrolling.

**URL and state fidelity**

- Search, filter, tab, and selected-item state is reflected in the URL (query params or path) so the page is shareable and refresh-safe.
- Validated search schemas (e.g., `validateSearch: (search) => schema.parse(search)`) fail loud when malformed, not silently.
- Dialogs and modals: optional, but if modal-as-route is used, the URL should change so deep-linking and back-button close the modal.
- Refreshing the page does not lose in-progress work without warning — either persist to query cache, warn on `beforeunload`, or clearly communicate "you'll lose X if you leave".

**Navigation affordances**

- Breadcrumbs on multi-step or nested pages; `PageHeader breadcrumb` is the canonical home for them in this app.
- "Back to X" link on detail pages when the user could get stuck (e.g., archived week detail → back to archive).
- Tabs inside a page change the URL (or at least `?tab=`), so copy-pasting the URL lands you on the same tab.
- Keyboard Enter on a form submits without forcing a mouse trip to the primary button.

**Rating guide**

- **5**: every click lands exactly where expected, back/forward/refresh all work correctly, URL is shareable, focus and scroll handled.
- **4**: minor — a missing breadcrumb on a deep page, or Cancel goes to root instead of restoring search.
- **3**: functional but some URL drift (filter state not in URL, modal state not in URL).
- **2**: at least one click destination surprises users, or back button loses work.
- **1**: page navigation is broken — save navigates before server confirms, or the user gets stuck on the page with no way to leave.

## Workflow probes you must run

Beyond rating each section, walk these probes deliberately — they catch the kind of bug that a section-by-section rating misses.

### Crossed-workflow probe

For every pair of workflows the page supports, run them in **non-default order** and check whether one silently clobbers the other. Examples of pairs:

- Stage an image, then apply a lookup candidate — does the image survive?
- Start editing, then change a parent select — does the edited data rebase or reset?
- Load a form, navigate to a subpage, come back — is the form state still there?
- Submit, get a validation error, fix one field, submit again — is the other error still shown?

Report findings even if the crossed order is rare — users *do* things in unusual order, and silent data loss is a P0.

### Input modality completeness

For any page that takes a piece of content (image, file, text, code, URL), enumerate *every* reasonable way the user might supply it. For an image, that includes: file upload, URL, paste from clipboard, drag-and-drop, **camera capture** on mobile, pick from a library. Any missing modality that a reasonable user would try in the next 30 days is a P1.

### Workflow dead-end probe

For every concept the form references (units, categories, tags, portions, variants), ask: can the user complete that concept on this page, or are they forced to save, leave, reopen, and try again? If the user has to save-before-configure, the workflow has a dead end. Dead ends are P1 when they gate a minority use case; P0 when they gate the happy path.

### Semantic search / translation probe

For each supported locale, run a real user-language query that should return a specific well-known item. If the search returns the wrong entity because the query was translated to the wrong term (e.g., German "Paprika" → English "paprika" instead of "bell pepper"), that's a data-quality bug surfaced through the page. Flag it under *Search & Filter* and, if the translation happens server-side, note the backend root-cause path too.

### Multi-locale skim

Switch locale once during the review (via settings or the language toggle) and skim the page. Layout issues (German words blowing past button widths, truncation, awkward line breaks) surface here, not in English-only review. Give this 2 minutes; it routinely finds P2s.

### Entry-point discovery probe

When a user lands on a "create" or "start" page in its **blank state**, the page is implicitly telling them how the task should be done. The question is: does the page acknowledge the *other* ways the same goal can be reached in this app?

For every create/new page, enumerate — by searching the routes directory and the navigation tree — every alternative way the same entity can be brought into existence. Common paths include:

- **Import** routes (`/import`, `/share-target`): paste a URL, upload a file, receive an Android share intent
- **Lookup** panels: auto-fill from an external database (OFF, FDC, AI)
- **Copy-from-existing** flows: clone a previous entity, duplicate a template
- **Templates / presets**: start from a canonical example
- **Camera / scan**: barcode, OCR, photo-to-recipe
- **Bulk / CSV** imports
- **Variants**: branch from an existing entity instead of starting fresh

Then ask two things:

1. **Is each alternative visible on this blank state?** Not just "reachable from somewhere in the app" — visible *here*, where the user has signalled they want to start. A hidden kebab menu on the list page does not count, because the user already walked past that menu when they clicked "New".
2. **Is the blank state teaching the user to use the alternative when it is a better choice?** A user who will hand-type 15 ingredients does not know that pasting a URL would have produced the same recipe in two seconds. The blank state is the highest-leverage place to route them.

Findings from this probe are usually P1 or P2 (not P0, because they don't block the happy path) but they are among the highest-value fixes — they convert the blank state from "a form to fill out" into "a launchpad with the fastest path first".

If the page has *no* alternative paths, say so explicitly in the review — the absence is a legitimate outcome and deserves to be documented so reviewers of later pages don't re-litigate it.

### Shared-component page-specific probe

When a component is reused across pages (e.g. `ImageField`, `LookupPanel`, a shared `Combobox`), do not defer findings to another review with "same as page X". Test the component **on the page being reviewed**, in the workflows this page drives. A shared component can behave correctly on one page and poorly on another because of context — different aspect ratios, different entity types, different defaults, different surrounding affordances.

For image inputs specifically, capture on *this* page: upload, URL, paste, camera (mobile), drag-drop, aspect-ratio handling, and any page-specific entity integration (e.g., do lookup candidates on this page populate an image? do duplicate/variant flows inherit it?). Even when the code is shared, the review is per-page.


## Overall Production-Readiness Score

After rating all sections, compute an **overall score** as the lowest of the sectional scores (not the average). A single section at 1 means the page is not ready to ship, even if everything else is at 5. The average is always misleading because a11y blockers and hardcoded strings can't be offset by beautiful animations.

State the overall score and the section(s) that drove it.
