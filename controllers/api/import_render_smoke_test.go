//go:build linux

package api

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestRenderedImportRealChromium renders public pages through the real render
// path with the Chromium of the host. It runs only where the environment was
// prepared for it: DARKPHISH_RENDER_PROFILE_ROOT on a small tmpfs inside a
// delegated cgroup, as scripts/render-smoke.sh and the "Rendered import smoke"
// workflow set up. Nothing else in the suite starts a browser, which is how a
// renderer limit that aborted Chromium 158 passed every unit test.
func TestRenderedImportRealChromium(t *testing.T) {
	if strings.TrimSpace(os.Getenv(renderedProfileRootEnvironment)) == "" {
		t.Skip("DARKPHISH_RENDER_PROFILE_ROOT is not set; scripts/render-smoke.sh prepares the environment")
	}
	chrome := chromiumExecutable()
	if chrome == "" {
		t.Fatal("no Chromium on this host: install one or set DARKPHISH_CHROMIUM_PATH")
	}
	// The wrapper records the environment the product hands to Chromium and
	// then becomes Chromium.
	dump := filepath.Join(t.TempDir(), "chromium-environment")
	wrapper := filepath.Join(t.TempDir(), "chromium")
	script := "#!/bin/sh\n/usr/bin/env -0 > " + dump + "\nexec " + chrome + " \"$@\"\n"
	if err := os.WriteFile(wrapper, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DARKPHISH_CHROMIUM_PATH", wrapper)
	t.Setenv("DARKPHISH_SECRET_ENCRYPTION_KEY", "render-smoke-sentinel")

	stop := make(chan struct{})
	peaks := make(chan string, 1)
	go sampleRenderPeaks(stop, peaks)

	for _, target := range []struct{ url, marker string }{
		{"https://example.com/", "Example Domain"},
		{"https://www.wikipedia.org/", "Wikipedia"},
	} {
		start := time.Now()
		rendered, finalURL, err := renderImportPage(context.Background(), target.url)
		if err != nil {
			t.Fatalf("%s: render failed after %s: %v (the render path needs a delegated cgroup and a tmpfs profile root; scripts/render-smoke.sh prepares both)", target.url, time.Since(start).Round(time.Millisecond), err)
		}
		if finalURL == nil || finalURL.Scheme != "https" {
			t.Fatalf("%s: final URL %v is not HTTPS", target.url, finalURL)
		}
		if !strings.Contains(string(rendered), target.marker) {
			t.Fatalf("%s: %d rendered bytes do not contain %q", target.url, len(rendered), target.marker)
		}
		t.Logf("%s: rendered %d bytes in %s", target.url, len(rendered), time.Since(start).Round(time.Millisecond))
	}
	close(stop)
	t.Logf("render peaks: %s", <-peaks)

	data, err := os.ReadFile(dump)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range strings.Split(strings.TrimRight(string(data), "\x00"), "\x00") {
		if name, _, ok := strings.Cut(entry, "="); ok && name != "" {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		t.Fatal("Chromium did not start through the wrapper")
	}
	for _, name := range names {
		if strings.HasPrefix(name, "DARKPHISH_") {
			t.Errorf("Chromium received %s", name)
		}
	}
	t.Logf("Chromium started with %d variables: %s", len(names), strings.Join(names, " "))
}

// sampleRenderPeaks reports the highest thread count and memory use the render
// cgroups reached while renders were running. Kernels older than 6.8 do not
// expose the peak files; the report then says unknown.
func sampleRenderPeaks(stop <-chan struct{}, result chan<- string) {
	self, _ := os.ReadFile("/proc/self/cgroup")
	_, group, _ := strings.Cut(string(self), "::")
	group = "/sys/fs/cgroup" + strings.TrimSpace(group)
	var threads, memory int
	for {
		select {
		case <-stop:
			if threads == 0 {
				result <- "unknown on this kernel"
			} else {
				result <- strconv.Itoa(threads) + " threads, " + strconv.Itoa(memory>>20) + " MiB"
			}
			return
		default:
		}
		for _, dir := range globRenderCgroups(group) {
			threads = max(threads, readCounter(filepath.Join(dir, "pids.peak")))
			memory = max(memory, readCounter(filepath.Join(dir, "memory.peak")))
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func globRenderCgroups(group string) []string {
	dirs, _ := filepath.Glob(filepath.Join(group, "darkphish-render-*"))
	return dirs
}

func readCounter(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	value, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	return value
}
