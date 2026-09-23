//go:build ignore

package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/scullxbones/armature/internal/validate"
)

func main() {
	here, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	root := filepath.Clean(filepath.Join(here, "..", ".."))
	doc, err := os.ReadFile(filepath.Join(root, "docs", "design", "surface-census.md"))
	if err != nil {
		log.Fatal(err)
	}
	src, err := validate.RenderCensusedSurfacesGo(string(doc))
	if err != nil {
		log.Fatal(err)
	}
	out := filepath.Join(here, "censused_surfaces_gen.go")
	if err := os.WriteFile(out, []byte(src), 0o644); err != nil {
		log.Fatal(err)
	}
}
