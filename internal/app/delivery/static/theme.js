// Тема valet: светлая или тёмная. Подключается в <head> без defer, чтобы тема встала до первой отрисовки
// и экран не мигал светлым перед тёмным.
//
// Пока пользователь не трогал тумблер, тема как в системе и меняется вместе с ней. Переключил - выбор
// хранится в localStorage этого устройства. Не на сервере: офлайн-копии страниц сохранены целиком,
// и серверный выбор расходился бы с ними.
"use strict";

(() => {
  const KEY = "valet-theme";
  const root = document.documentElement;
  const system = window.matchMedia("(prefers-color-scheme: dark)");
  // полоса браузера над страницей - цвета бумаги темы, см. app.css
  const barColors = { light: "#F3EDDF", dark: "#141C2E" };

  // chosen - тема, которую выбрал пользователь, или null. localStorage бывает недоступен (приватный режим)
  const chosen = () => {
    try {
      const theme = localStorage.getItem(KEY);
      return theme === "light" || theme === "dark" ? theme : null;
    } catch {
      return null;
    }
  };

  const current = () => chosen() ?? (system.matches ? "dark" : "light");

  const apply = () => {
    const theme = current();
    root.dataset.theme = theme;
    // у экрана кассы полоса своя, светлая в любой теме: её meta без data-theme-color
    for (const meta of document.querySelectorAll("meta[data-theme-color]")) {
      meta.content = barColors[theme];
    }
    document.dispatchEvent(new CustomEvent("valet:theme", { detail: theme }));
  };

  apply();
  system.addEventListener("change", () => {
    if (!chosen()) apply();
  });

  // valetTheme - для тумблера в app.js
  window.valetTheme = {
    current,
    set(theme) {
      try {
        localStorage.setItem(KEY, theme);
      } catch {
        // не сохранилось - тема всё равно сменится на этой странице
      }
      apply();
    },
  };
})();
