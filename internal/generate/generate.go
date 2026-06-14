// Package generate turns a validated spec into the on-disk artifacts that make
// up a client environment: site config + data + content skeleton, Caddy site
// blocks, the CRM env + compose snippet, and a residual-steps checklist.
package generate

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/howarthTech/legion-post-platform/internal/spec"
)

//go:embed all:templates
var templatesFS embed.FS

// Result carries the one-time secrets back to the caller so they can be
// surfaced in the checklist (and nowhere else).
type Result struct {
	OutDir        string
	AdminUsername string
	AdminPassword string // plaintext, shown once
	Files         []string
}

// Run generates everything for s into outRoot/<client>/ and returns a Result.
func Run(s *spec.Spec, outRoot string) (*Result, error) {
	dir := filepath.Join(outRoot, s.Client)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}

	// One-time CRM secrets.
	sessionSecret, err := RandomSecret(32)
	if err != nil {
		return nil, err
	}
	adminPassword, err := RandomPassword(12)
	if err != nil {
		return nil, err
	}
	adminHash, err := BcryptHash(adminPassword)
	if err != nil {
		return nil, err
	}

	// Template data: the spec plus the derived secrets.
	data := map[string]any{
		"Spec":          s,
		"SessionSecret": sessionSecret,
		"AdminHash":     adminHash,
		"AdminPassword": adminPassword, // checklist only
	}

	res := &Result{
		OutDir:        dir,
		AdminUsername: s.AdminUsername,
		AdminPassword: adminPassword,
	}

	// (template path, output path relative to dir)
	jobs := []struct{ tmpl, out string }{
		{"templates/hugo.toml.tmpl", "site/hugo.toml"},
		{"templates/officers.yaml.tmpl", "site/data/officers.yaml"},
		{"templates/site.caddy.tmpl", fmt.Sprintf("caddy/%s.caddy", s.Domain)},
		{"templates/admin.caddy.tmpl", fmt.Sprintf("caddy/admin.%s.caddy", s.Domain)},
		{"templates/crm.env.tmpl", fmt.Sprintf("crm/%s.env", s.Client)},
		{"templates/docker-compose.snippet.yml.tmpl", "crm/docker-compose.snippet.yml"},
		{"templates/CHECKLIST.md.tmpl", "CHECKLIST.md"},
	}
	for _, j := range jobs {
		if err := renderFile(j.tmpl, filepath.Join(dir, j.out), data); err != nil {
			return nil, fmt.Errorf("render %s: %w", j.tmpl, err)
		}
		res.Files = append(res.Files, j.out)
	}

	// Content skeleton: the standard page set as stubs with TODO markers.
	files, err := scaffoldContent(filepath.Join(dir, "site", "content"), s)
	if err != nil {
		return nil, fmt.Errorf("scaffold content: %w", err)
	}
	res.Files = append(res.Files, files...)

	return res, nil
}

var funcs = template.FuncMap{
	"telDigits": func(p string) string {
		return strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' {
				return r
			}
			return -1
		}, p)
	},
}

func renderFile(tmplPath, outPath string, data any) error {
	t, err := template.New(filepath.Base(tmplPath)).Funcs(funcs).ParseFS(templatesFS, tmplPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()
	// CRM env file holds secrets — tighten perms.
	if strings.HasSuffix(outPath, ".env") {
		_ = f.Chmod(0o600)
	}
	return t.Execute(f, data)
}
