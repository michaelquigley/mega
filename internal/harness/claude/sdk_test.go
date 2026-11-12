package claude

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSDKVersionIsTheInstalledPackages(t *testing.T) {
	shim := t.TempDir()
	h := New(Config{Shim: shim})
	if _, err := h.SDKVersion(); err == nil {
		t.Error("no sdk installed: want an error")
	}
	dir := filepath.Join(shim, "node_modules", "@anthropic-ai", "claude-agent-sdk")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"@anthropic-ai/claude-agent-sdk","version":"0.3.318","type":"module"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := h.SDKVersion(); err != nil || got != "0.3.318" {
		t.Errorf("sdk version: %q, %v", got, err)
	}
	if _, err := New(Config{}).SDKVersion(); err == nil {
		t.Error("no shim configured: want an error")
	}
}
