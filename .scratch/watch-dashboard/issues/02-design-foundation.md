# 02: Design foundation and page shell

**What to build:** The designer's deliverables (DESIGN.md §12) become the app's base styling. This covers design tokens as CSS custom properties (dark mode only), the type and spacing scales, fonts if any, the favicon, and the page shell: a header, the Movies / TV shows switch, and space for the search, sort and refresh controls. The Movies page from 01 shows its title and year in the designed movie-row layout. Later tickets fill in the rest of each component using these tokens.

**Blocked by:** 01 (Walking skeleton). The designer's deliverables are in `designs/`.

**Status:** ready-for-agent

- [ ] `tokens.css` is copied from `designs/tokens.css` with `color-scheme: dark` and without the light-theme block; the reduced-motion block stays and zeroes `--duration`
- [ ] The component layout, view-model pattern and gallery test from CLAUDE.md "UI components" are set up, with the `poster` placeholder, `button` and `movie-row` (title, year, added date) as the first components
- [ ] The header matches the design: the Spool mark and name, the Movies / TV shows tabs with counts, and space for the refresh control
- [ ] Instrument Sans and IBM Plex Mono woff2 files are embedded with their SIL OFL licence, and the system font stacks are the fallback (if downloading is blocked by the network policy, report the host that needs allowing)
- [ ] The Movies / TV shows switch is present and works with the keyboard; the TV shows tab can be a placeholder page for now
- [ ] The movie row matches the design at desktop width and at 380 px, without horizontal page scroll
- [ ] Every interactive element has a visible focus style; motion respects `prefers-reduced-motion`
- [ ] Text meets WCAG 2.2 AA contrast
- [ ] The Spool reel favicon is a single SVG, and it is embedded and served
- [ ] All wording uses sentence case (DESIGN.md §11)
