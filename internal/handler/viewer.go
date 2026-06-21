package handler

import (
	"embed"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
)

//go:embed viewer/*
var viewerFiles embed.FS

// viewerFS is the sub-filesystem rooted at viewer/.
var viewerFS fs.FS

func init() {
	var err error
	viewerFS, err = fs.Sub(viewerFiles, "viewer")
	if err != nil {
		panic("failed to initialize viewer filesystem: " + err.Error())
	}
}

// ViewerHandler serves the embedded SPA viewer files.
type ViewerHandler struct{}

// NewViewerHandler creates a new ViewerHandler that serves the embedded viewer files.
func NewViewerHandler() (*ViewerHandler, error) {
	return &ViewerHandler{}, nil
}

// ServeHTTP implements http.Handler.
// Serves index.html for root; delegates other paths to the file server.
func (h *ViewerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}

	// Try to open the requested file
	f, err := viewerFS.Open(path)
	if err != nil {
		// SPA fallback: serve index.html for unknown paths
		slog.Debug("viewer: file not found, serving index.html", "path", r.URL.Path)
		f, err = viewerFS.Open("index.html")
		if err != nil {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if stat.IsDir() {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	// Serve the file content directly — avoids FileServer redirect issues
	buf, err := io.ReadAll(f)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Detect content type
	contentType := "text/html; charset=utf-8"
	if strings.HasSuffix(path, ".css") {
		contentType = "text/css; charset=utf-8"
	} else if strings.HasSuffix(path, ".js") {
		contentType = "application/javascript; charset=utf-8"
	} else if strings.HasSuffix(path, ".json") {
		contentType = "application/json"
	} else if strings.HasSuffix(path, ".svg") {
		contentType = "image/svg+xml"
	}

	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	w.Write(buf)
}
