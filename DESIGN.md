# Smart Search setup interface

This page is a small local utility. Its job is to prepare a title catalogue,
connect an installed Stremio client and explain how to keep the helper running.
Design for someone who wants to finish setup and return to watching.

## Reference

Visual direction informed by [Evreghen Command Center by frknaykc on
designmd.ai](https://designmd.ai/frknaykc/command-center), listed under MIT.
The useful ideas here are the warm workspace, dark framing, system typography
and compact controls. The implementation and project-specific rules below are
original to Smart Search; no source code, fonts or images from that kit are bundled.

## Composition

- One centred utility, at most 1056px wide. A 272px catalogue sidebar frames a
  larger installation workspace. Stack the catalogue first on narrow screens.
- Catalogue counts and readiness come from the running helper. Do not show
  fabricated performance scores, install progress or success states.
- Installation has three steps: choose a client, paste its link, reorder the
  addon. Keep alternative web/portable instructions in an expandable disclosure.
- End with a small launch action and example queries. Put version and quit in
  the footer. There is no need for navigation to nonexistent screens.

## Palette

| Role | Value |
| --- | --- |
| Page | `#f1f0eb` |
| Workspace | `#fcfbf8` |
| Text | `#292d29` |
| Secondary text | `#6b7068` |
| Divider | `#dedfd7` |
| Catalogue panel | `#252d29` |
| Panel secondary text | `#b2bdb5` |
| Existing mint accent | `#8ce1c6` |
| Focus outline | `#238465` |

Use mint for the catalogue action and ready indicator. Client choices are neutral
rows with a clear hover border. Status must always have text as well as colour.

## Type and controls

- Use installed system UI fonts, with a monospace stack only for addresses,
  numbered steps and example queries. Load no remote assets.
- Main heading: 27px. Section titles: 14px. Body: 13px. Supporting metadata:
  11–12px. Use ordinary sentence case in instructions.
- Use 4/8px spacing increments, 24–34px panel padding, 5px control corners and
  10px outer corners. Keep the page flat, with borders for separation.
- Controls must remain keyboard accessible with visible focus outlines.
  Disabled clients explain whether the app was detected and whether the
  catalogue is ready. Preserve clipboard failure instructions.
- The mobile layout retains download, privacy and background-running guidance.
  Respect reduced-motion preferences for the progress indicator.

## Copy

Use short instructions that describe the next action. Keep the explicit HTTP
address and port guidance: some clients otherwise treat localhost as HTTPS.
Describe local matching as a benefit without promising a universal speedup.
Explain that moving this search-only addon preserves the other providers'
relative order, and that account order may sync to other devices.

Avoid advertising headlines, decorative badges, gradients, glass effects,
oversized cards and repeated assurances. Every visible element should help
someone set up or understand the helper.
