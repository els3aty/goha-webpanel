package ui

import (
	"embed"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
)

//go:embed dist/*
var distFS embed.FS

// DistFS returns an http.FileSystem serving the embedded React frontend.
func DistFS() http.FileSystem {
	f, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err)
	}
	return http.FS(f)
}

// SPAServer implements http.Handler for a single page application.
// If a file is not found, it serves index.html so React Router can handle it.
type SPAServer struct {
	fs http.FileSystem
}

func NewSPAServer() *SPAServer {
	return &SPAServer{
		fs: DistFS(),
	}
}

func (s *SPAServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Try to open the requested file
	f, err := s.fs.Open(r.URL.Path)
	if os.IsNotExist(err) || r.URL.Path == "/" {
		// Not found or root, serve index.html
		r.URL.Path = "/index.html"
		f, err = s.fs.Open("/index.html")
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer f.Close()

	// Get FileInfo to serve the file properly
	fi, err := f.Stat()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Serve the file
	http.ServeContent(w, r, fi.Name(), fi.ModTime(), f)
}
