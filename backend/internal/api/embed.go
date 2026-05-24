package api

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// embeddedFrontend is the frontend tree baked into the binary at build time.
// The path is relative to this Go source file: ../../../frontend resolves
// to /home/carson/openzerg/frontend at compile.
//
// IMPORTANT: the //go:embed directive must reference the directory by a
// relative path; the build fails loudly if the directory is empty.
//
//go:embed all:frontend_embed
var embeddedFrontend embed.FS

// embeddedRoot is the subdirectory name inside the embed. We mirror the
// repo's frontend/ tree into backend/internal/api/frontend_embed at build
// time via a small build step (handled by build-frontend.sh). Doing it this
// way keeps the //go:embed path next to the .go source per Go's rules.
const embeddedRoot = "frontend_embed"

// buildFrontendHandler returns the http.Handler that serves either an
// on-disk directory (dev) or the embed.FS (production / demo). It also
// implements the SPA fallback (unknown paths -> index.html) and the
// per-asset Cache-Control headers.
func buildFrontendHandler(dirOverride string) (http.Handler, error) {
	if dirOverride != "" {
		info, err := os.Stat(dirOverride)
		if err != nil {
			return nil, fmt.Errorf("frontend: --frontend %q: %w", dirOverride, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("frontend: %q is not a directory", dirOverride)
		}
		indexPath := filepath.Join(dirOverride, "index.html")
		if _, err := os.Stat(indexPath); err != nil {
			return nil, fmt.Errorf("frontend: missing index.html in %q", dirOverride)
		}
		return &diskFrontendHandler{root: dirOverride}, nil
	}

	subFS, err := fs.Sub(embeddedFrontend, embeddedRoot)
	if err != nil {
		return nil, fmt.Errorf("frontend: embed sub: %w", err)
	}
	if _, err := fs.Stat(subFS, "index.html"); err != nil {
		return nil, errors.New(
			"frontend: embedded tree is missing index.html — rebuild the binary " +
				"after running scripts/build-frontend.sh (M7)")
	}
	return &embedFrontendHandler{fsys: subFS}, nil
}

type diskFrontendHandler struct {
	root string
}

func (h *diskFrontendHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	urlPath := path.Clean("/" + strings.TrimPrefix(r.URL.Path, "/"))
	candidate := filepath.Join(h.root, filepath.FromSlash(strings.TrimPrefix(urlPath, "/")))
	if urlPath == "/" || candidate == h.root {
		serveDiskFile(w, r, filepath.Join(h.root, "index.html"), true)
		return
	}
	info, err := os.Stat(candidate)
	if err != nil || info.IsDir() {
		serveDiskFile(w, r, filepath.Join(h.root, "index.html"), true)
		return
	}
	serveDiskFile(w, r, candidate, false)
}

func serveDiskFile(w http.ResponseWriter, r *http.Request, fullPath string, isIndex bool) {
	if isIndex {
		w.Header().Set("Cache-Control", "no-store")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=300")
	}
	http.ServeFile(w, r, fullPath)
}

type embedFrontendHandler struct {
	fsys fs.FS
}

func (h *embedFrontendHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	urlPath := strings.TrimPrefix(r.URL.Path, "/")
	if urlPath == "" {
		h.serveEmbedFile(w, r, "index.html", true)
		return
	}
	if _, err := fs.Stat(h.fsys, urlPath); err == nil {
		h.serveEmbedFile(w, r, urlPath, false)
		return
	}
	// SPA fallback: unknown path -> index.html so client-side router can
	// render. Never falls back for /api/* — those are routed before this
	// handler via the http.ServeMux precedence rules.
	h.serveEmbedFile(w, r, "index.html", true)
}

func (h *embedFrontendHandler) serveEmbedFile(w http.ResponseWriter, r *http.Request, name string, isIndex bool) {
	data, err := fs.ReadFile(h.fsys, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if isIndex {
		w.Header().Set("Cache-Control", "no-store")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=300")
	}
	w.Header().Set("Content-Type", contentTypeFor(name))
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	_, _ = w.Write(data)
}

func contentTypeFor(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".html":
		return "text/html; charset=utf-8"
	case ".js", ".mjs":
		return "application/javascript; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".svg":
		return "image/svg+xml"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".json":
		return "application/json; charset=utf-8"
	case ".ico":
		return "image/x-icon"
	default:
		return "application/octet-stream"
	}
}
