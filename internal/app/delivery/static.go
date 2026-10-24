package delivery

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
)

// assetHashLen - сколько символов sha256 оставлять в ?v=, для различения версий хватает с запасом
const assetHashLen = 12

const (
	// cacheImmutable - для адресов с актуальной версией: содержимое по ним никогда не меняется
	cacheImmutable string = "public, max-age=31536000, immutable"
	// cacheRevalidate - браузер хранит файл, но перед использованием сверяет ETag
	cacheRevalidate string = "no-cache"
)

// assets - путь файла внутри static/ -> хэш его содержимого
type assets map[string]string

// hashAssets - считает хэши всех файлов статики; вызывается один раз при старте
func hashAssets(static fs.FS) (assets, error) {
	res := make(assets)
	err := fs.WalkDir(static, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}

		data, err := fs.ReadFile(static, path)
		if err != nil {
			return fmt.Errorf("can't read %s: %w", path, err)
		}

		sum := sha256.Sum256(data)
		res[path] = hex.EncodeToString(sum[:])[:assetHashLen]

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("can't hash static files: %w", err)
	}

	return res, nil
}

// url - адрес файла с версией: /static/app.css?v=3f9a1c2e5b7d
func (a assets) url(path string) (string, error) {
	hash, ok := a[path]
	if !ok {
		return "", fmt.Errorf("unknown static file: %s", path)
	}

	return "/static/" + path + "?v=" + url.QueryEscape(hash), nil
}

// handler - раздаёт статику. Адрес с актуальной версией неизменен, его кэшируем на год.
// Остальные (без ?v= или со старой версией, например шрифты из fonts.css) браузер перепроверяет
// по ETag и при неизменном файле получает 304 без тела
func (a assets) handler(static fs.FS) http.Handler {
	files := http.FileServer(http.FS(static))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hash, ok := a[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}

		// FileServer сам ответит 304 на If-None-Match, если ETag выставлен заранее
		w.Header().Set("ETag", `"`+hash+`"`)
		if r.URL.Query().Get("v") == hash {
			w.Header().Set("Cache-Control", cacheImmutable)
		} else {
			w.Header().Set("Cache-Control", cacheRevalidate)
		}

		files.ServeHTTP(w, r)
	})
}
