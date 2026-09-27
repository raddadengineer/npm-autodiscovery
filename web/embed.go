package web

import (
	"embed"
	"io/fs"
)

//go:embed public/*
var embeddedFiles embed.FS

// GetFS returns an fs.FS sub-filesystem anchored at "public".
func GetFS() (fs.FS, error) {
	return fs.Sub(embeddedFiles, "public")
}
