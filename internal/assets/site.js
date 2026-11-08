(() => {
  const themeKey = "vanitypages-theme";
  const body = document.body;

  document.querySelector("[data-theme-toggle]")?.addEventListener("click", () => {
    const dark = body.classList.contains("g-root_theme_dark");
    const next = dark ? "light" : "dark";
    body.classList.replace(dark ? "g-root_theme_dark" : "g-root_theme_light", `g-root_theme_${next}`);
    try {
      localStorage.setItem(themeKey, next);
    } catch {
      // Storage can be unavailable; the choice just won't persist.
    }
  });

  for (const button of document.querySelectorAll("[data-copy]")) {
    let timer;
    button.addEventListener("click", async () => {
      await navigator.clipboard.writeText(button.parentElement.querySelector("code").textContent);
      button.classList.add("vp-copied");
      clearTimeout(timer);
      timer = setTimeout(() => button.classList.remove("vp-copied"), 1500);
    });
  }
})();
