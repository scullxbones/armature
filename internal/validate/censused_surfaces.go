package validate

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

//go:generate go run generate_censused_surfaces.go

const surfaceCensusRelPath = "docs/design/surface-census.md"

func parseCensusedSurfaceTable(doc string) map[string][]string {
	out := make(map[string][]string)
	inSection := false
	for _, line := range strings.Split(doc, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			inSection = trimmed == "## Censused Surfaces"
			continue
		}
		if !inSection || !strings.HasPrefix(trimmed, "|") {
			continue
		}
		cells := strings.Split(strings.Trim(trimmed, "|"), "|")
		if len(cells) < 2 {
			continue
		}
		surface := strings.Trim(strings.TrimSpace(cells[0]), "`")
		if surface == "" || surface == "Surface" || strings.HasPrefix(surface, "---") {
			continue
		}
		var docFiles []string
		for _, f := range strings.Split(cells[1], ",") {
			if cleaned := strings.Trim(strings.TrimSpace(f), "`"); cleaned != "" {
				docFiles = append(docFiles, cleaned)
			}
		}
		out[surface] = docFiles
	}
	return out
}

// RenderCensusedSurfacesGo renders censused_surfaces_gen.go from the census doc table.
func RenderCensusedSurfacesGo(doc string) (string, error) {
	m := parseCensusedSurfaceTable(doc)
	if len(m) == 0 {
		return "", fmt.Errorf("no Censused Surfaces table in %s", surfaceCensusRelPath)
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString("// Code generated from docs/design/surface-census.md; DO NOT EDIT.\n\n")
	b.WriteString("package validate\n\n")
	b.WriteString("var censusedSurfaces = map[string][]string{\n")
	for _, k := range keys {
		b.WriteString("\t")
		b.WriteString(strconv.Quote(k))
		b.WriteString(": {")
		for i, f := range m[k] {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(strconv.Quote(f))
		}
		b.WriteString("},\n")
	}
	b.WriteString("}\n")
	return b.String(), nil
}
