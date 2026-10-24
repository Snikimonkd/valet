package delivery

import (
	"io/fs"
	"net/http"
)

// serviceWorker - sw.js из статики, но по адресу в корне: service worker управляет только адресами
// не выше своего. Версии в адресе у него нет, поэтому браузер каждый раз сверяет его по ETag
func (wb *delivery) serviceWorker(static fs.FS) http.HandlerFunc {
	return wb.rootFile(static, "sw.js", "text/javascript; charset=utf-8")
}

// manifest - описание PWA: имя, иконки и цвета для установки на главный экран.
// theme_color чёрный: им установленное PWA на Android красит полосу жестов внизу экрана, страница
// на неё не влияет. Полосу браузера сверху красит meta theme-color страницы, см. theme.js
func (wb *delivery) manifest(static fs.FS) http.HandlerFunc {
	return wb.rootFile(static, "manifest.webmanifest", "application/manifest+json")
}

func (wb *delivery) rootFile(static fs.FS, name, contentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := fs.ReadFile(static, name)
		if err != nil {
			wb.serverError(w, r, err)
			return
		}

		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", cacheRevalidate)
		w.Header().Set("ETag", `"`+wb.assets[name]+`"`)
		if r.Header.Get("If-None-Match") == `"`+wb.assets[name]+`"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		_, _ = w.Write(data)
	}
}

// ping - жива ли сессия. app.js по ответу понимает, пришла страница из офлайн-кэша из-за сети
// или из-за истёкшей сессии. Сам ответ пустой, кэшировать его нельзя
func (wb *delivery) ping(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}
