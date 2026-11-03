// Package conformance holds cross-package architectural guards. Unlike the
// decode conformance suite in this directory, these tests do not decode
// anything: they assert the import graph of the public pkg/ packages so the
// layering that keeps the decoder lean cannot silently regress.
//
// The intended acyclic layering is:
//
//	pkg/av1   (leaf: shared vocabulary)
//	pkg/rtp   -> av1
//	pkg/decode-> av1, rtp
//	pkg/encode-> av1, rtp, internal/av1/encoder
//
// In particular a decode-only consumer (go-media's thumbnail path) must never
// link the encoder, and the shared vocabulary must stay free of codec logic.
package conformance

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const modulePrefix = "github.com/gtsteffaniak/go-av1"

// TestPublicPackageLayering asserts, for each pkg/ package, that its transitive
// dependency closure excludes the packages it must not reach.
func TestPublicPackageLayering(t *testing.T) {
	cases := []struct {
		pkg       string
		forbidden []string
	}{
		{
			// Shared vocabulary is the bottom of the graph: no codec logic.
			pkg: "./pkg/av1",
			forbidden: []string{
				modulePrefix + "/pkg/decode",
				modulePrefix + "/pkg/encode",
				modulePrefix + "/pkg/rtp",
				modulePrefix + "/internal/av1/encoder",
			},
		},
		{
			// RTP wire formats are packet plumbing only, never codec entry points.
			pkg: "./pkg/rtp",
			forbidden: []string{
				modulePrefix + "/pkg/decode",
				modulePrefix + "/pkg/encode",
				modulePrefix + "/internal/av1/encoder",
			},
		},
		{
			// The decode path must not drag in the encoder or the encoder-facing
			// package. This is the size-critical guard from the restructure plan.
			pkg: "./pkg/decode",
			forbidden: []string{
				modulePrefix + "/pkg/encode",
				modulePrefix + "/internal/av1/encoder",
			},
		},
		{
			// The encoder is the top of the graph; it must never reach back into
			// the decoder.
			pkg: "./pkg/encode",
			forbidden: []string{
				modulePrefix + "/pkg/decode",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.pkg, func(t *testing.T) {
			deps := packageDeps(t, tc.pkg)
			for _, bad := range tc.forbidden {
				if _, ok := deps[bad]; ok {
					t.Errorf("%s must not depend on %s (import layering violated)", tc.pkg, bad)
				}
			}
		})
	}
}

// packageDeps returns the set of transitive package import paths for pkg by
// shelling out to "go list -deps", which is the same view the linker uses.
func packageDeps(t *testing.T, pkg string) map[string]struct{} {
	t.Helper()
	cmd := exec.Command("go", "list", "-deps", pkg)
	cmd.Dir = repoRoot(t)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -deps %s: %v", pkg, err)
	}
	deps := make(map[string]struct{})
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		deps[line] = struct{}{}
	}
	return deps
}

// repoRoot walks up from the test's working directory (the conformance package
// directory) to the module root, identified by go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no go.mod found above %s", dir)
		}
		dir = parent
	}
}
