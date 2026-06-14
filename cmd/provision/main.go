// provision — generate a complete Legion post environment from a client spec.
//
//	provision -spec clients/post-5.yaml [-out out]
//
// Reads and validates the spec, generates the site config + data + content
// skeleton, Caddy site blocks, CRM env + compose snippet, and an onboarding
// checklist into out/<client>/. Per-client secrets (admin password, session
// secret) are generated fresh; the admin password is printed once.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/howarthTech/legion-post-platform/internal/generate"
	"github.com/howarthTech/legion-post-platform/internal/spec"
)

func main() {
	specPath := flag.String("spec", "", "path to the client spec YAML (required)")
	outRoot := flag.String("out", "out", "output root directory")
	flag.Parse()

	if *specPath == "" {
		fmt.Fprintln(os.Stderr, "usage: provision -spec <client.yaml> [-out <dir>]")
		flag.PrintDefaults()
		os.Exit(2)
	}

	s, err := spec.Load(*specPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "spec error: %v\n", err)
		os.Exit(1)
	}

	res, err := generate.Run(s, *outRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "generate error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Provisioned %s (%s)\n", s.PostName, s.Domain)
	fmt.Printf("  Output: %s/\n", res.OutDir)
	fmt.Printf("  Files:  %d generated\n", len(res.Files))
	fmt.Println()
	fmt.Println("  ── One-time admin credentials (save now) ──────────────")
	fmt.Printf("    CRM login: %s / %s\n", res.AdminUsername, res.AdminPassword)
	fmt.Println("  ───────────────────────────────────────────────────────")
	fmt.Println()
	fmt.Printf("  Next: read %s/CHECKLIST.md for the manual steps.\n", res.OutDir)
}
