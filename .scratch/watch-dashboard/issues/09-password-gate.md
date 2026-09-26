# 09: Optional password gate

**What to build:** When `DASHBOARD_PASSWORD` is set, the dashboard asks for it before showing anything, using the designed password screen. When it isn't set, there is no gate.

**Blocked by:** 02 (Design foundation and page shell)

**Status:** ready-for-agent

- [ ] With `DASHBOARD_PASSWORD` unset, every page loads with no prompt
- [ ] With it set, every page, fragment and image route redirects (or returns 401 for htmx requests) until a session exists
- [ ] The password is compared in constant time; a wrong password shows the designed error state
- [ ] The session cookie is HttpOnly and SameSite
- [ ] The password never appears in logs
- [ ] Tests cover the gated and ungated modes, wrong and right passwords, and protected image routes
