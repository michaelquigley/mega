package ui

import (
	"io/fs"
	"regexp"
	"sort"
	"strings"
)

// assetRef finds the content-hashed assets an index.html loads.
var assetRef = regexp.MustCompile(`(?:src|href)="(/assets/[^"]+)"`)

// Build is the identity of the embedded interface: the content-hashed assets
// its index.html loads, sorted and joined by spaces. it changes exactly when
// the interface's code or style does, and a page computes the same from the
// document it was served, so an open page can tell that the service now
// serves another interface. it is empty when no interface is embedded.
func Build() string {
	index, err := fs.ReadFile(FS, "dist/index.html")
	if err != nil {
		return ""
	}
	return buildOf(index)
}

func buildOf(index []byte) string {
	var assets []string
	for _, match := range assetRef.FindAllSubmatch(index, -1) {
		assets = append(assets, string(match[1]))
	}
	sort.Strings(assets)
	return strings.Join(assets, " ")
}
