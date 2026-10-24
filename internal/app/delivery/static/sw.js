// Service worker valet: офлайн-копии карт, чтобы на кассе без сети карта всё равно открылась.
//
// Два кэша:
// - STATIC - шрифты, стили, скрипты. Не персональные, переживают выход;
// - CARDS - список карт (/), страницы карт (/cards/<id>) и картинки кодов. Персональные:
//   стираются, когда пользователь выходит (POST /logout проходит через этот service worker).
// Формы добавления и правки не кэшируются: без сети их всё равно не отправить.
//
// Страницы: сначала сеть, кэш - только если сети нет или сервер не пустил
// (сессия истекла и он отправил на вход). Статика и картинки кодов - сразу из кэша:
// их адреса содержат версию, по одному адресу содержимое не меняется.

const STATIC = "valet-static-v1";
const CARDS = "valet-cards-v1";

// страницы, которые нужны офлайн
const offlinePage = /^\/(cards\/\d+)?$/;
// картинка кода карты с версией в адресе
const qrImage = /^\/cards\/\d+\/qr\.png$/;

self.addEventListener("install", () => self.skipWaiting());

self.addEventListener("activate", (event) => {
  event.waitUntil(
    (async () => {
      // кэши прошлых версий service worker больше не нужны
      for (const name of await caches.keys()) {
        if (name !== STATIC && name !== CARDS) await caches.delete(name);
      }
      await self.clients.claim();
    })(),
  );
});

self.addEventListener("fetch", (event) => {
  const req = event.request;
  const url = new URL(req.url);
  if (url.origin !== self.location.origin) return;

  if (req.method === "POST" && url.pathname === "/logout") {
    event.respondWith(logout(req));
    return;
  }
  if (req.method !== "GET") return;

  if (url.pathname.startsWith("/static/")) {
    event.respondWith(cacheFirst(STATIC, req));
    return;
  }
  if (qrImage.test(url.pathname)) {
    event.respondWith(cacheFirst(CARDS, req));
    return;
  }
  if (offlinePage.test(url.pathname)) {
    event.respondWith(networkFirst(req));
  }
  // остальное (вход, формы, /ping) идёт в сеть как обычно
});

// logout - выход: после него карт на телефоне остаться не должно. Удаляется только кэш карт,
// статика остаётся, поэтому выход мгновенный и следующая страница не перекачивает шрифты
async function logout(req) {
  await caches.delete(CARDS);
  return fetch(req);
}

// cacheFirst - из кэша name, а если там нет - из сети с сохранением в него.
// Новая версия файла приходит по новому адресу (?v=...), старые версии того же файла удаляются,
// иначе кэш рос бы с каждым релизом
async function cacheFirst(name, req) {
  const cached = await caches.match(req, { cacheName: name });
  if (cached) return cached;

  const res = await fetch(req);
  if (res.ok) {
    const cache = await caches.open(name);
    const path = new URL(req.url).pathname;
    for (const old of await cache.keys()) {
      if (new URL(old.url).pathname === path) await cache.delete(old);
    }
    await cache.put(req, res.clone());
  }
  return res;
}

// networkFirst - из сети с обновлением кэша. Из кэша - если сети нет или вместо страницы пришёл
// редирект на вход: на кассе протухшая сессия не должна прятать карту
async function networkFirst(req) {
  let res;
  try {
    res = await fetch(req);
  } catch {
    return (await caches.match(req, { cacheName: CARDS })) ?? offlineStub();
  }

  if (res.ok) {
    const cache = await caches.open(CARDS);
    await cache.put(req, res.clone());
    return res;
  }

  // у навигации редирект приходит непрозрачным ответом без статуса, у fetch из страницы - уже пройденным
  if (res.type === "opaqueredirect" || res.redirected) {
    const cached = await caches.match(req, { cacheName: CARDS });
    if (cached) return cached;
  }
  return res;
}

function offlineStub() {
  const html =
    '<!doctype html><meta charset="utf-8"><meta name="viewport" content="width=device-width">' +
    "<title>Нет сети</title><p>Нет сети, а этой страницы нет в сохранённых. " +
    '<a href="/">К списку карт</a></p>';
  return new Response(html, { status: 503, headers: { "Content-Type": "text/html; charset=utf-8" } });
}

// prefetch - список карт присылает адреса всех карт пользователя: кладём их в кэш заранее,
// чтобы офлайн открывалась и карта, которую на этом телефоне ещё ни разу не открывали.
// Карты, которых в списке больше нет (удалены), из кэша убираем
async function prefetch(urls) {
  const cache = await caches.open(CARDS);
  const keep = new Set(urls.map((u) => new URL(u, self.location.origin).href));

  for (const url of urls) {
    const isQR = qrImage.test(new URL(url, self.location.origin).pathname);
    // картинка по адресу с версией не меняется: если она уже есть, качать заново незачем
    if (isQR && (await cache.match(url))) continue;

    try {
      // заголовок говорит серверу, что это не открытие карты на кассе: карта не поднимется наверх списка
      const res = await fetch(url, { headers: { "X-Valet-Prefetch": "1" }, redirect: "error" });
      if (res.ok) await cache.put(url, res);
    } catch {
      // сеть пропала посреди обхода: что успели - сохранили, остальное докачаем в следующий раз
      return;
    }
  }

  for (const req of await cache.keys()) {
    if (!keep.has(req.url)) await cache.delete(req);
  }
}

self.addEventListener("message", (event) => {
  if (event.data?.type === "prefetch" && Array.isArray(event.data.urls)) {
    event.waitUntil(prefetch(event.data.urls));
  }
});
