# Setup page design

The settings page follows [LongCipher Design by Akagi201](https://designmd.ai/Akagi201/longcipher-design)
(CC0), a Vercel-style system. This page keeps the parts of that reference that apply
to a small Go/HTML/CSS/JavaScript utility, plus the rules specific to Smart Search.

## Look

- White workspace (`#ffffff`) with near-black text (`#171717`).
- Shadow borders instead of CSS borders, for example
  `rgba(0,0,0,0.08) 0px 0px 0px 1px, rgba(0,0,0,0.04) 0px 2px 2px`.
  Child corner radii are no larger than their parent's.
- Blue focus indicator (`hsla(212, 100%, 48%, 1)`), shown with `:focus-visible`.
  Hover, active and focus states have more contrast than the rest state.
- Never use colour alone for status; always include a text label.

## Type

- Geist Sans and Geist Mono, bundled locally under the SIL Open Font License. No
  remote fonts or other UI assets.
- Tight, never positive, letter-spacing on Geist Sans. Headings stay at utility
  scale, not the reference's marketing display sizes.
- Tabular numbers (`font-variant-numeric: tabular-nums`) for counts, sizes and
  timings.

## Copy

- Active voice, second person: "Prepare catalogue", not "The catalogue will be prepared".
- Specific button labels that name the action, such as **Apply size & rebuild**.
- Errors say how to fix the problem, not just what went wrong.
- Numerals for counts, a real ellipsis (`…`) for labels that open a dialog, such as
  **Change location…**.

## Interaction

- Links for navigation and buttons for actions.
- Label and control share one hit target of at least 24 px. Inputs are at least 16 px
  so mobile browsers do not zoom.
- Never block paste or typing; validate and show feedback instead.
- Keep a submit button enabled until work starts, then disable it, show progress and
  keep its label.
- Use `aria-live="polite"` for progress and status updates, `aria-label` on
  icon-only buttons, and `alt` text on images.
- Animate only `transform` and `opacity`, never `transition: all`, and respect
  `prefers-reduced-motion`.

## Smart Search rules

- A catalogue size or alternative-title change takes effect only after an explicit,
  successful rebuild. The working catalogue stays available during preparation and
  after a failure.
- Show actual counts, size limits and measurement context. Link to the
  [performance](PERFORMANCE.md) measurements instead of promising universal speed or
  memory figures.
- Keep the client-specific HTTP clipboard install flow, the addon-order guidance and
  the explanation that the helper keeps running in the background.
