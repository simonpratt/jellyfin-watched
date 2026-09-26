# 09: Optional password gate

**What to build:** When `DASHBOARD_PASSWORD` is set, the dashboard asks for it before showing anything, using the designed password screen. When it isn't set, there is no gate.

**Blocked by:** 02 (Design foundation and page shell)

**Status:** done

- [x] With `DASHBOARD_PASSWORD` unset, every page loads with no prompt
- [x] With it set, every page, fragment and image route redirects (or returns 401 for htmx requests) until a session exists
- [x] The password is compared in constant time; a wrong password shows the designed error state
- [x] The session cookie is HttpOnly and SameSite
- [x] The password never appears in logs
- [x] Tests cover the gated and ungated modes, wrong and right passwords, and protected image routes
