# 02: Design foundation and page shell

**What to build:** The designer's deliverables (DESIGN.md §12) become the app's base styling. This covers design tokens as CSS custom properties for the light and dark themes (chosen from the OS setting), the type and spacing scales, fonts if any, the favicon, and the page shell: a header, the Movies / TV shows switch, and space for the search, sort and refresh controls. The Movies page from 01 shows its title and year in the designed movie-row layout. Later tickets fill in the rest of each component using these tokens.

**Blocked by:** 01 (Walking skeleton), plus the designer's deliverables

**Status:** ready-for-agent

- [ ] Tokens from the design are implemented as CSS custom properties, with both themes following `prefers-color-scheme`
- [ ] Any bundled fonts are self-hosted and embedded, and their licence is recorded in the repo
- [ ] The Movies / TV shows switch is present and works with the keyboard; the TV shows tab can be a placeholder page for now
- [ ] The movie row matches the design at desktop width and at 380 px, without horizontal page scroll
- [ ] Every interactive element has a visible focus style; motion respects `prefers-reduced-motion`
- [ ] Text meets WCAG 2.2 AA contrast in both themes
- [ ] The favicon (if one was designed) is embedded and served
- [ ] All wording uses sentence case (DESIGN.md §11)
