package harness

import (
	"cmp"
	"slices"
	"strings"

	"github.com/michaelquigley/mega/internal/model"
)

// FirstLine is a tool description's first non-empty line, trimmed: the one
// line the interface shows.
func FirstLine(description string) string {
	for _, line := range strings.Split(description, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return ""
}

// SortMCP orders servers, and each server's tools, by name, so a list the
// harness gives in a different order each time reads the same.
func SortMCP(servers []model.MCPServer) {
	slices.SortFunc(servers, func(a, b model.MCPServer) int { return cmp.Compare(a.Name, b.Name) })
	for i := range servers {
		slices.SortFunc(servers[i].Tools, func(a, b model.MCPTool) int { return cmp.Compare(a.Name, b.Name) })
	}
}
