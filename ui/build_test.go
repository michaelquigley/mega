package ui

import "testing"

func TestBuildIsTheAssetsTheIndexLoads(t *testing.T) {
	index := []byte(`<!doctype html>
<html><head>
<link rel="icon" type="image/svg+xml" href="/favicon.svg" />
<script type="module" crossorigin src="/assets/index-BrPGccIy.js"></script>
<link rel="stylesheet" crossorigin href="/assets/index-XszsJHD-.css">
</head><body></body></html>`)
	if got, want := buildOf(index), "/assets/index-BrPGccIy.js /assets/index-XszsJHD-.css"; got != want {
		t.Errorf("build: %q, want %q", got, want)
	}
	if got := buildOf([]byte("<html></html>")); got != "" {
		t.Errorf("no assets: %q", got)
	}
}
