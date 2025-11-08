package webserver

import (
	"fmt"
	"net/http"

	"readeckobo/internal/app"
	"readeckobo/internal/logger"
)

// ListenAndServe starts the HTTP server on the specified port.
func ListenAndServe(port int, application *app.App, logger *logger.Logger) {
	addr := fmt.Sprintf(":%d", port)
	logger.Infof("Web server starting on port %s", addr)

	mux := http.NewServeMux()

	// Register handlers
	mux.HandleFunc("/api/kobo/get", application.HandleKoboGet)
	mux.HandleFunc("/api/kobo/download", application.HandleKoboDownload)
	mux.HandleFunc("/api/kobo/send", application.HandleKoboSend)
	mux.HandleFunc("/api/convert-image", application.HandleConvertImage)
	mux.HandleFunc("/instapaper-proxy/storeapi/v1/initialization", application.HandleDumpAndForward)

	// Catch-all for unimplemented routes
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		logger.Warnf("404 Not Found: URL=%s, Method=%s, Params=%v", r.URL.Path, r.Method, r.URL.Query())
		http.Error(w, "404 Not Found", http.StatusNotFound)
	})

	// Apply logging middleware
	loggedMux := LoggingMiddleware(mux)

	if err := http.ListenAndServe(addr, loggedMux); err != nil {
		logger.Errorf("Web server failed to start: %v", err)
	}
}