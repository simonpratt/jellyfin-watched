// Jellyfin-watched's client glue, loaded by the layout. Templates carry only
// hx-* attributes; behaviour lives here.
(function () {
  "use strict";

  // A failed htmx request shows its message in the layout's #error-slot,
  // and the next request that succeeds hides it. Server errors are
  // text/plain.
  function slot() {
    return document.getElementById("error-slot");
  }

  function showError(message) {
    var s = slot();
    if (!s) return;
    s.querySelector("[data-error-text]").textContent = message;
    s.hidden = false;
  }

  document.addEventListener("htmx:responseError", function (e) {
    var xhr = e.detail.xhr;
    var plain = (xhr.getResponseHeader("Content-Type") || "").indexOf("text/plain") === 0;
    var text = plain ? xhr.responseText.trim().slice(0, 300) : "";
    showError(text || "Something went wrong (HTTP " + xhr.status + "). Try again.");
  });

  document.addEventListener("htmx:sendError", function () {
    showError("Couldn't reach Jellyfin-watched. Check it's running, then try again.");
  });

  document.addEventListener("htmx:afterRequest", function (e) {
    var s = slot();
    if (s && e.detail.successful) s.hidden = true;
  });
})();
