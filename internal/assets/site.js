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

  const viewSwitch = document.querySelector("[data-view-switch]");
  if (viewSwitch) {
    const syncViewSwitch = () => {
      const view = body.classList.contains("vp-view-list") ? "list" : "cards";
      for (const input of viewSwitch.querySelectorAll("input")) {
        input.checked = input.value === view;
        input.parentElement.classList.toggle("g-segmented-radio-group__option_checked", input.checked);
      }
    };
    viewSwitch.addEventListener("change", (event) => {
      body.classList.toggle("vp-view-list", event.target.value === "list");
      try {
        localStorage.setItem("vanitypages-view", event.target.value);
      } catch {
        // Storage can be unavailable; the choice just won't persist.
      }
      syncViewSwitch();
    });
    syncViewSwitch();
  }

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
