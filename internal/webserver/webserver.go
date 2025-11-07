package webserver

import (
	"fmt"
	"log"
	"net/http"

	"readeckobo/internal/app"
)

// ListenAndServe starts the HTTP server on the specified port.
func ListenAndServe(port int, application *app.App) {
	addr := fmt.Sprintf(":%d", port)
	log.Printf("Web server starting on port %s", addr)

	mux := http.NewServeMux()

	// Register handlers
	mux.HandleFunc("/api/kobo/get", application.HandleKoboGet)
	mux.HandleFunc("/api/kobo/download", application.HandleKoboDownload)
	mux.HandleFunc("/api/kobo/send", application.HandleKoboSend)
	mux.HandleFunc("/api/convert-image", application.HandleConvertImage)

	// Catch-all for unimplemented routes
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("404 Not Found: URL=%s, Method=%s, Params=%v", r.URL.Path, r.Method, r.URL.Query())
		http.Error(w, "404 Not Found", http.StatusNotFound)
	})

	// Apply logging middleware
	loggedMux := LoggingMiddleware(mux)

	if err := http.ListenAndServe(addr, loggedMux); err != nil {
		log.Fatalf("Web server failed to start: %v", err)
	}
}