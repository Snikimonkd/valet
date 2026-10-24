// valet: немного поведения поверх серверного html. Без этого файла всё работает,
// кроме офлайн-режима, фильтра по списку и запрета гасить экран на кассе.
"use strict";

// service worker кладёт карты в кэш, чтобы они открывались без сети, см. sw.js
if ("serviceWorker" in navigator) {
  navigator.serviceWorker.register("/sw.js").catch(() => {});
}

// prefetchCards - отдаёт service worker адреса всех карт со страницы списка: он заранее кладёт их в кэш
async function prefetchCards() {
  const urls = [...document.querySelectorAll("[data-offline]")].flatMap((el) => el.dataset.offline.split(" "));
  if (!urls.length || !("serviceWorker" in navigator) || !navigator.onLine) return;

  const reg = await navigator.serviceWorker.ready;
  reg.active?.postMessage({ type: "prefetch", urls: ["/", ...urls] });
}

// normalize - поиск без учёта регистра и разницы между «е» и «ё»
const normalize = (s) => s.toLocaleLowerCase("ru").replaceAll("ё", "е").trim();

// filterCards - фильтр списка по названию прямо в браузере: карты уже на странице,
// так что он работает и без сети
function filterCards(input) {
  const query = normalize(input.value);
  let shown = 0;
  for (const card of document.querySelectorAll("[data-card-name]")) {
    const match = normalize(card.dataset.cardName).includes(query);
    card.hidden = !match;
    if (match) shown++;
  }
  const empty = document.querySelector("[data-filter-empty]");
  if (empty) empty.hidden = shown > 0;
}

// keepScreenOn - пока на экране карта, телефон не гасит экран: кассир может долго возиться со сканером.
// Браузер снимает запрет, когда вкладка уходит в фон, поэтому при возвращении просим снова
function keepScreenOn() {
  if (!("wakeLock" in navigator)) return;

  const request = () => navigator.wakeLock.request("screen").catch(() => {});
  request();
  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState === "visible") request();
  });
}

// setOnline - что показать с сетью и без неё. Без сети - баннер «только просмотр», а кнопки с data-online-only
// (правка, удаление, добавление, выход) серые и не нажимаются: их страниц нет в офлайн-кэше.
// Вернулась сеть - всё снова доступно
function setOnline(online) {
  const banner = document.querySelector("[data-offline-banner]");
  if (banner) banner.hidden = online;

  for (const el of document.querySelectorAll("[data-online-only]")) {
    if (online) {
      el.removeAttribute("aria-disabled");
    } else {
      el.setAttribute("aria-disabled", "true");
    }
    if (el instanceof HTMLButtonElement) el.disabled = !online;
  }
}

// выключенную ссылку браузер всё равно откроет, а data-dialog откроет окно: нажатие гасим раньше всех,
// ещё на пути к кнопке (capture), чтобы до её собственных обработчиков оно не дошло
document.addEventListener(
  "click",
  (event) => {
    if (event.target.closest?.('[data-online-only][aria-disabled="true"]')) {
      event.preventDefault();
      event.stopImmediatePropagation();
    }
  },
  true,
);

// checkConnection - есть ли сеть и жива ли сессия. navigator.onLine врёт, когда Wi-Fi подключён, а интернета нет,
// поэтому спрашиваем сервер. Страница могла прийти из офлайн-кэша, и он подскажет, почему:
// нет сети - setOnline(false), сессия истекла - баннер со ссылкой на вход
async function checkConnection() {
  const relogin = document.querySelector("[data-relogin-banner]");
  const needed = relogin || document.querySelector("[data-offline-banner], [data-online-only]");
  if (!needed) return;

  // без сети точно: выключаем сразу, не дожидаясь ответа
  if (!navigator.onLine) {
    setOnline(false);
    return;
  }

  try {
    const res = await fetch("/ping", { cache: "no-store" });
    setOnline(true);
    const loggedOut = res.redirected && new URL(res.url).pathname === "/login";
    if (relogin) relogin.hidden = !loggedOut;
  } catch {
    setOnline(false);
  }
}

// confirmDialogs - кнопка с data-dialog открывает подтверждение окном поверх страницы вместо перехода
// на отдельную. Окно закрывается кнопкой в нём, Esc или нажатием на затемнение вокруг
function confirmDialogs() {
  for (const opener of document.querySelectorAll("[data-dialog]")) {
    const dialog = document.getElementById(opener.dataset.dialog);
    if (!dialog?.showModal) continue;

    opener.addEventListener("click", (event) => {
      event.preventDefault();
      dialog.showModal();
    });
    // у окна нет своих отступов (см. .modal), поэтому нажатие точно на него - это нажатие мимо содержимого
    dialog.addEventListener("click", (event) => {
      if (event.target === dialog) dialog.close();
    });
  }
}

document.addEventListener("DOMContentLoaded", () => {
  confirmDialogs();

  const filter = document.querySelector("[data-filter]");
  if (filter) {
    filter.addEventListener("input", () => filterCards(filter));
    // браузер мог восстановить текст в поле после «назад»
    if (filter.value) filterCards(filter);
  }

  if (document.body.dataset.page === "card") keepScreenOn();
  if (document.body.dataset.page === "cards") prefetchCards();

  checkConnection();
  window.addEventListener("online", checkConnection);
  window.addEventListener("offline", checkConnection);

  // выбор файла сразу отправляет форму распознавания: лишняя кнопка «Загрузить» на телефоне ни к чему.
  // Кнопка остаётся для браузеров без js, этот скрипт её прячет
  for (const input of document.querySelectorAll("[data-autosubmit]")) {
    input.form.querySelector("[data-autosubmit-button]")?.setAttribute("hidden", "");
    input.addEventListener("change", () => {
      if (input.files.length) input.form.requestSubmit();
    });
  }
});
