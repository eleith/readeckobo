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
	if err := http.ListenAndServe(addr, NewHandler(application, logger)); err != nil {
		logger.Errorf("Web server failed to start: %v", err)
	}
}

func NewHandler(application *app.App, logger *logger.Logger) http.Handler {
	mux := http.NewServeMux()

	// Register handlers
	mux.HandleFunc("/api/kobo/get", application.HandleKoboGet)
	mux.HandleFunc("/api/kobo/download", application.HandleKoboDownload)
	mux.HandleFunc("/api/kobo/send", application.HandleKoboSend)
	mux.HandleFunc("/api/convert-image", application.HandleConvertImage)
	mux.HandleFunc("/instapaper-proxy/instapaper/api/kobo/get", application.HandleKoboGet)
	mux.HandleFunc("/instapaper-proxy/instapaper/api/kobo/download", application.HandleKoboDownload)
	mux.HandleFunc("/instapaper-proxy/instapaper/api/kobo/send", application.HandleKoboSend)
	storeProxy := http.StripPrefix("/instapaper-proxy/storeapi", http.HandlerFunc(application.HandleStoreProxy))
	mux.Handle("/instapaper-proxy/storeapi/", storeProxy)
	mux.Handle("/instapaper-proxy/storeapi", storeProxy)

	// Catch-all for unimplemented routes
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		logger.Warnf("404 Not Found: Method=%s", r.Method)
		http.Error(w, "404 Not Found", http.StatusNotFound)
	})

	return LoggingMiddleware(mux)
}
