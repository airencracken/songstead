// Keep save feedback in view after full-page and HTMX form submissions.
(() => {
  function focusSaved() {
    const status = document.querySelector('[data-save-status]');
    if (status) status.focus();
  }
  document.addEventListener('DOMContentLoaded', focusSaved);
  document.addEventListener('htmx:afterSettle', focusSaved);
  // A previous confirmation must not look like confirmation of an unsaved edit.
  function clearSaved(event) {
    const form = event.target.closest('form');
    if (!form) return;
    for (const status of form.querySelectorAll('[data-save-status]')) status.remove();
  }
  document.addEventListener('input', clearSaved);
  document.addEventListener('change', clearSaved);
})();
