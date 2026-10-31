package api

import (
	"net/http"
	"os"
	"path/filepath"
)

// StaticHandler отдаёт собранную веб-панель (SPA) из папки dir.
//
// Если запрошенный путь — реальный файл (например, /assets/index-abc.js),
// отдаём его; иначе отдаём index.html, чтобы работал клиентский роутинг.
// Запросы к /api/*, /healthz, /metrics и webhook перехватываются более
// конкретными маршрутами ServeMux и сюда не попадают.
func StaticHandler(dir string) http.HandlerFunc {
	fileServer := http.FileServer(http.Dir(dir))
	return func(w http.ResponseWriter, r *http.Request) {
		// filepath.Clean + корень защищают от выхода за пределы dir.
		path := filepath.Join(dir, filepath.Clean("/"+r.URL.Path))
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			fileServer.ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(dir, "index.html"))
	}
}
