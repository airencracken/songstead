// Music previews use authenticated same-origin requests and local artwork only.
(() => {
  const active = new Set();
  function compose(form) {
    if (form.dataset.previewBound) return;
    form.dataset.previewBound = '1';
    const input = form.elements.url, panel = form.querySelector('[data-link-preview]');
    let timer, controller, objectURL, generation = 0;
    function clearImage() {
      if (objectURL) URL.revokeObjectURL(objectURL);
      objectURL = null;
      const image = panel.querySelector('[data-preview-artwork]');
      image.hidden = true; image.removeAttribute('src');
    }
    function reset() {
      generation++; clearTimeout(timer); controller?.abort(); clearImage();
      panel.hidden = true;
      for (const name of ['title','artist']) {
        const field = form.elements[name];
        if (field.value === field.dataset.autoValue) field.value = '';
        delete field.dataset.autoValue;
      }
    }
    async function preview(version, raw) {
      if (!form.isConnected || version !== generation) return;
      panel.hidden = false;
      const status = panel.querySelector('[data-preview-status]');
      status.textContent = 'Fetching music preview…';
      panel.querySelector('[data-preview-title]').textContent = '';
      panel.querySelector('[data-preview-artist]').textContent = '';
      controller = new AbortController();
      try {
        const response = await fetch('/recommendations/preview', {method:'POST', credentials:'same-origin',
          headers:{'Content-Type':'application/x-www-form-urlencoded','X-CSRF-Token':form.elements.csrf.value},
          body:new URLSearchParams({url:raw}),signal:controller.signal});
        if (version !== generation || !form.isConnected) return;
        if (!response.ok) throw new Error('Preview unavailable');
        const data = await response.json();
        if (version !== generation || !form.isConnected) return;
        if (data.status !== 'ready') {
          status.textContent = data.status === 'unsupported' ? 'No automatic preview for this link. Add a title if you like.' : 'Preview unavailable right now. You can still share this link.';
          return;
        }
        panel.querySelector('[data-preview-title]').textContent = data.title || '';
        panel.querySelector('[data-preview-artist]').textContent = data.artist || '';
        for (const [name,value] of [['title',data.title],['artist',data.artist]]) {
          const field = form.elements[name];
          if (!field.value && value && value !== raw && [...value].length <= 160) { field.value = value; field.dataset.autoValue = value; }
        }
        if (data.artwork) {
          const bytes = Uint8Array.from(atob(data.artwork), char => char.charCodeAt(0));
          objectURL = URL.createObjectURL(new Blob([bytes],{type:'image/png'}));
          const image = panel.querySelector('[data-preview-artwork]'); image.src = objectURL; image.hidden = false;
        }
        status.textContent = data.artwork ? 'Music preview ready.' : 'Music details found; artwork is unavailable right now.';
      } catch (error) {
        if (version === generation && error.name !== 'AbortError' && form.isConnected) status.textContent = 'Preview unavailable right now. You can still share this link.';
      }
    }
    for (const name of ['title','artist']) form.elements[name].addEventListener('input', () => { delete form.elements[name].dataset.autoValue; });
    input.addEventListener('input', () => {
      reset();
      if (input.value && input.validity.valid) timer = setTimeout(() => preview(generation,input.value),650);
    });
    if (input.value && input.validity.valid) timer = setTimeout(() => preview(generation,input.value),650);
    const cleanup = () => { reset(); active.delete(cleanup); };
    active.add(cleanup);
  }
  function card(element) {
    if (element.dataset.previewBound || element.querySelector('img[src$="/thumbnail"]')) return;
    element.dataset.previewBound = '1';
    let attempts = 0, timer, controller;
    const cleanup = () => { clearTimeout(timer); controller?.abort(); active.delete(cleanup); };
    active.add(cleanup);
    async function refresh() {
      if (!element.isConnected || ++attempts > 20) { cleanup(); return; }
      controller = new AbortController();
      try {
        const response = await fetch(`/recommendations/${element.dataset.musicCard}/preview`,{credentials:'same-origin',signal:controller.signal});
        if (!response.ok) { cleanup(); return; }
        const data = await response.json();
        if (!element.isConnected) { cleanup(); return; }
        const title = element.querySelector('[data-music-title]'), artist = element.querySelector('[data-music-artist]');
        if (title) title.textContent = data.title;
        if (artist) artist.textContent = data.artist;
        if (data.image) {
          const holder = element.querySelector('[data-artwork-holder]');
          if (holder) {
            const image = document.createElement('img'); image.src = data.image; image.alt = ''; image.width = 320; image.height = 240;
            if (element.classList.contains('detail')) image.className = 'detail-artwork';
            holder.replaceChildren(image);
          }
          const retry = element.querySelector('[data-music-status]')?.closest('form'); if (retry) retry.remove();
          cleanup(); return;
        }
        if (!data.pending) {
          const status = element.querySelector('[data-music-status]');
          if (status) status.textContent = 'No artwork is available. You can retry supported links or open the original music link.';
          cleanup(); return;
        }
        timer = setTimeout(refresh,3000);
      } catch { cleanup(); }
    }
    void refresh();
  }
  function initialize() {
    document.querySelectorAll('[data-music-compose]').forEach(compose);
    document.querySelectorAll('[data-music-card]').forEach(card);
  }
  document.addEventListener('DOMContentLoaded',initialize);
  document.addEventListener('htmx:afterSettle',initialize);
  document.addEventListener('htmx:beforeSwap',event => {
    if (event.detail?.shouldSwap) for (const cleanup of [...active]) cleanup();
  });
  window.addEventListener('pagehide',() => { for (const cleanup of [...active]) cleanup(); });
})();
