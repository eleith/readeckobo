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

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("Web server failed to start: %v", err)
	}
}
