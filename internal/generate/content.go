package generate

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/howarthTech/legion-post-platform/internal/spec"
)

// scaffoldContent writes the standard page skeleton for a new instance. Pages
// that are post-specific (history, rental, officers, family programs) are
// written as stubs carrying the theme's {{< todo >}} marker so they're visibly
// incomplete in staging but hidden in production. The operator/post fills the
// real text.
//
// Pages whose content is generic Legion boilerplate (flag etiquette, why-join,
// membership requirements) are intentionally NOT scaffolded here in v1 — the
// recommended flow is to copy those from the Post 5 reference instance, since
// they're identical across posts. The checklist calls this out.
func scaffoldContent(contentDir string, s *spec.Spec) ([]string, error) {
	var written []string

	pages := map[string]string{
		"_index.md": frontMatter("American Legion "+s.PostShortName,
			s.PostName+" — "+s.Locality+", "+s.RegionLong+"."),

		"about/_index.md": stub("About "+s.PostShortName,
			"Who we are, where we meet, and how we serve "+s.Locality+".",
			"Write the post's introduction. Officers render automatically from data/officers.yaml via the {{</* officers */>}} shortcode."),

		"about/history.md": stub("Post History",
			"The history of "+s.PostName+".",
			"Have the Post Historian write the post's history — founding year, namesake, milestones."),

		"membership/_index.md": stub("Membership",
			"Eligibility, dues, and how to join "+s.PostShortName+".",
			"Membership overview. Copy the standard eligibility/why-join/apply pages from the reference instance and adjust dues if different."),

		"events/_index.md":        eventsIndex(),
		"events/_content.gotmpl":  eventsAdapter(),

		"family/_index.md": stub("The American Legion Family",
			"The Auxiliary, Sons of the American Legion, and Legion Riders at "+s.PostShortName+".",
			"Intro to the post's Legion Family. Family contacts render from data/officers.yaml via {{</* family-contacts */>}}."),

		"rental/_index.md": stub("Hall Rental",
			"Rent our facility for your event.",
			"Add rental pricing, capacity, amenities, photos, and a booking contact."),

		"gallery/_index.md": stub("Photo Gallery",
			"Photos from "+s.PostShortName+" events.",
			"Add albums under content/gallery/ as page bundles (a folder of photos + index.md). See the reference instance for an example."),

		"contact.md": contactPage(s),
	}

	for rel, body := range pages {
		out := filepath.Join(contentDir, rel)
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(out, []byte(body), 0o644); err != nil {
			return nil, err
		}
		written = append(written, filepath.Join("site/content", rel))
	}
	return written, nil
}

func frontMatter(title, desc string) string {
	return fmt.Sprintf("---\ntitle: %q\ndescription: %q\n---\n", title, desc)
}

func stub(title, desc, todo string) string {
	return fmt.Sprintf("---\ntitle: %q\ndescription: %q\n---\n\n{{< todo >}}\n%s\n{{< /todo >}}\n",
		title, desc, todo)
}

func eventsIndex() string {
	return `---
title: "Events"
description: "Upcoming meetings and past gatherings."
outputs:
  - HTML
  - RSS
  - EventsJSON
  - EventsICS
cascade:
  outputs:
    - HTML
    - EventCal
---

Events are authored in the post's CRM (the Events screen), not in this repo.
The content adapter next to this file builds the pages from the CRM's public
events API at build time.
`
}

// eventsAdapter is the Hugo content adapter that builds event pages from the
// client CRM's public /api/events.json (params.eventsAPI in hugo.toml). Kept
// in sync with the reference instance's content/events/_content.gotmpl.
func eventsAdapter() string {
	return `{{/* Build event pages from the CRM's events API (the CRM is the source of
     truth — events are authored by the post's admin in the CRM, not in this
     repo). URL set in hugo.toml [params] eventsAPI.

     Failure mode is deliberate: if the API is unreachable or malformed, the
     BUILD FAILS (errorf) rather than silently publishing a site with no
     events. The deploy job only rsyncs successful builds, so the live site
     keeps its last good event pages. */}}

{{ $url := site.Params.eventsAPI }}
{{ if not $url }}
  {{ errorf "params.eventsAPI is not set — the events section cannot build" }}
{{ end }}

{{ $opts := dict "headers" (dict "User-Agent" "hugo-legion-site-build") }}
{{ with resources.GetRemote $url $opts }}
  {{ with .Err }}
    {{ errorf "events API fetch failed (%s): %s" $url . }}
  {{ else }}
    {{ $feed := .Content | transform.Unmarshal }}
    {{ range $feed.events }}
      {{ $params := dict
           "location"     (.location | default "")
           "contactName"  (.contactName | default "")
           "contactPhone" (.contactPhone | default "")
           "description"  (.description | default "")
           "eventType"    (.type | default "post")
      }}
      {{ with .endsAt }}{{ $params = merge $params (dict "endDate" .) }}{{ end }}
      {{ $.AddPage (dict
           "path"    .slug
           "title"   .title
           "dates"   (dict "date" (time.AsTime .startsAt))
           "params"  $params
           "content" (dict "mediaType" "text/markdown" "value" (.body | default ""))
      ) }}
    {{ end }}
  {{ end }}
{{ else }}
  {{ errorf "events API returned no response (%s)" $url }}
{{ end }}
`
}

func contactPage(s *spec.Spec) string {
	return fmt.Sprintf(`---
title: "Contact %s"
description: "How to reach the officers and the Post."
---

## By Email or Phone

> **Email:** <a href="mailto:%s">%s</a>
> **Phone:** {{< phone "%s" >}}
> **Mail:** %s

{{< todo >}}
Add the contact form (copy the form block + form-flash shortcode from the
reference instance's contact.md) once this post's Resend sender is set up.
{{< /todo >}}
`, s.PostShortName, s.Email, s.Email, s.Phone, s.MailingAddress)
}
