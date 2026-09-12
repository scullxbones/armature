//go:build ignore

package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/scullxbones/armature/internal/doctor"
)

func main() {
	here, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	root := filepath.Clean(filepath.Join(here, "..", ".."))
	if err := doctor.WriteCheckIDsDoc(root); err != nil {
		log.Fatal(err)
	}
}
