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

		"gallery/_index.md":       galleryIndex(s),
		"gallery/_content.gotmpl": galleryAdapter(),

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

func galleryIndex(s *spec.Spec) string {
	return fmt.Sprintf(`---
title: "Photo Gallery"
description: "Photos from %s events, ceremonies, and gatherings."
---

Browse photos from %s. Click any album below to view its photos.

Albums are managed in the post's CRM (the Photo gallery screen) and build into
this page automatically. The content adapter next to this file pulls each album
and its photos from the CRM's public gallery API at build time.
`, s.PostShortName, s.PostShortName)
}

// galleryAdapter is the Hugo content adapter that builds album pages from the
// client CRM's public /api/gallery.json (params.galleryAPI in hugo.toml). Each
// photo is fetched and attached as a page image resource so the theme's gallery
// layouts can thumbnail/resize it. Kept in sync with the reference instance's
// content/gallery/_content.gotmpl.
func galleryAdapter() string {
	return `{{/* Build gallery album pages from the CRM's gallery API (the CRM is the
     source of truth — albums and photos are managed by the post's admin in the
     CRM, not in this repo). URL set in hugo.toml [params] galleryAPI.

     Each album becomes a page bundle and each photo is attached as a page
     image resource, so the theme's gallery layouts can thumbnail and resize it.

     Failure mode is deliberate, matching events: if the API (or any photo) is
     unreachable or malformed, the BUILD FAILS (errorf) rather than silently
     publishing an incomplete gallery. The deploy job only rsyncs successful
     builds, so the live site keeps its last good gallery. */}}

{{ $url := site.Params.galleryAPI }}
{{ if not $url }}
  {{ errorf "params.galleryAPI is not set — the gallery section cannot build" }}
{{ end }}

{{ $opts := dict "headers" (dict "User-Agent" "hugo-legion-site-build") }}
{{ with resources.GetRemote $url $opts }}
  {{ with .Err }}
    {{ errorf "gallery API fetch failed (%s): %s" $url . }}
  {{ else }}
    {{ $feed := .Content | transform.Unmarshal }}
    {{ range $feed.albums }}
      {{ $album := . }}
      {{ $date := .date | default (now.Format "2006-01-02") }}
      {{ $.AddPage (dict
           "path"    .slug
           "title"   .title
           "dates"   (dict "date" (time.AsTime $date))
           "content" (dict "mediaType" "text/markdown" "value" (.description | default ""))
      ) }}
      {{ range $i, $photo := .photos }}
        {{ with resources.GetRemote $photo.url $opts }}
          {{ with .Err }}
            {{ errorf "gallery photo fetch failed (%s): %s" $photo.url . }}
          {{ else }}
            {{ $.AddResource (dict
                 "path"    (printf "%s/%04d-%s" $album.slug $i (path.Base $photo.url))
                 "title"   ($photo.caption | default "")
                 "content" (dict "mediaType" .MediaType.Type "value" .Content)
            ) }}
          {{ end }}
        {{ else }}
          {{ errorf "gallery photo returned no response (%s)" $photo.url }}
        {{ end }}
      {{ end }}
    {{ end }}
  {{ end }}
{{ else }}
  {{ errorf "gallery API returned no response (%s)" $url }}
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
