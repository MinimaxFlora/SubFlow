export{};

// When a newly deployed service worker takes over from a previously cached one,
// reload once so the browser stops running the stale app shell. The guard skips
// the very first visit, where no controller exists yet.
if ("serviceWorker" in navigator && navigator.serviceWorker.controller) {
  let refreshing = false;
  navigator.serviceWorker.addEventListener("controllerchange", () => {
    if (refreshing) return;
    refreshing = true;
    window.location.reload();
  });
}

setTimeout(() => {
  import('./main').then(({ initializeApp }) => {
    initializeApp();
  });
}, 10);
