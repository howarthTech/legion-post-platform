// Package spec defines the per-client provisioning specification — the single
// YAML file that fully describes a Legion post tenant. The provisioner reads
// one of these and emits a complete environment (site config, officer data,
// content skeleton, Caddy blocks, CRM env, checklist).
package spec

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Spec is the whole client definition. Field comments document the YAML keys.
type Spec struct {
	Client        string  `yaml:"client"`        // slug: dirs/files/secrets names, e.g. "post-5"
	Domain        string  `yaml:"domain"`        // custom domain, e.g. "romelegion.org"
	PostName      string  `yaml:"postName"`      // "Shanklin Attaway Post 5"
	PostShortName string  `yaml:"postShortName"` // "Post 5"
	CharterYear   int     `yaml:"charterYear"`   // 1925
	Description   string  `yaml:"description"`   // meta description

	// Location / identity
	Locality    string `yaml:"locality"`    // city
	Region      string `yaml:"region"`      // state abbr
	RegionLong  string `yaml:"regionLong"`  // state full
	ServiceArea string `yaml:"serviceArea"` // "Rome and Floyd County"
	Timezone    string `yaml:"timezone"`    // IANA tz, e.g. "America/New_York"

	// Hero
	HeroTitle    string `yaml:"heroTitle"`    // homepage <h1> (HTML allowed)
	HeroImageAlt string `yaml:"heroImageAlt"` // hero image alt text

	// Contact
	Email          string `yaml:"email"`
	Phone          string `yaml:"phone"`
	FacebookURL    string `yaml:"facebookURL"`
	FacebookSearch string `yaml:"facebookSearch"`
	MailingAddress string `yaml:"mailingAddress"` // display string
	MeetingLocation string `yaml:"meetingLocation"` // display string
	MeetingSchedule string `yaml:"meetingSchedule"` // display string

	// Structured addresses (schema.org)
	Postal Address `yaml:"postal"`
	Venue  Venue   `yaml:"venue"`

	MapShortlinks map[string]string `yaml:"mapShortlinks"` // substring -> shortlink
	Brand         map[string]string `yaml:"brand"`         // token -> hex (optional)

	Officers       []Officer       `yaml:"officers"`
	FamilyContacts []FamilyContact `yaml:"familyContacts"`

	// CRM
	CRMPort      int    `yaml:"crmPort"`      // loopback port for this client's CRM container
	AdminUsername string `yaml:"adminUsername"` // CRM admin login (default "admin")
	SiteRepo     string `yaml:"siteRepo"`     // GitHub repo of the client's site (rebuild dispatch target)
}

type Address struct {
	Street     string `yaml:"street"`
	Locality   string `yaml:"locality"`
	Region     string `yaml:"region"`
	PostalCode string `yaml:"postalCode"`
}

type Venue struct {
	Name       string `yaml:"name"`
	Street     string `yaml:"street"`
	Locality   string `yaml:"locality"`
	Region     string `yaml:"region"`
	PostalCode string `yaml:"postalCode"`
}

type Officer struct {
	Role   string `yaml:"role"`
	Name   string `yaml:"name"`
	Phone  string `yaml:"phone"`
	Email  string `yaml:"email"`
	Weight int    `yaml:"weight"`
}

type FamilyContact struct {
	Role   string `yaml:"role"`
	Name   string `yaml:"name"`
	Phone  string `yaml:"phone"`
	Weight int    `yaml:"weight"`
}

var slugRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*[a-z0-9]$`)
var domainRE = regexp.MustCompile(`^[a-z0-9.-]+\.[a-z]{2,}$`)

// Load reads and validates a spec from a YAML file.
func Load(path string) (*Spec, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s Spec
	if err := yaml.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	s.applyDefaults()
	if err := s.Validate(); err != nil {
		return nil, err
	}
	return &s, nil
}

// usStates maps two-letter abbreviations to full names, so a spec (or the
// intake form) only has to supply `region` and we derive `regionLong`.
var usStates = map[string]string{
	"AL": "Alabama", "AK": "Alaska", "AZ": "Arizona", "AR": "Arkansas",
	"CA": "California", "CO": "Colorado", "CT": "Connecticut", "DE": "Delaware",
	"FL": "Florida", "GA": "Georgia", "HI": "Hawaii", "ID": "Idaho",
	"IL": "Illinois", "IN": "Indiana", "IA": "Iowa", "KS": "Kansas",
	"KY": "Kentucky", "LA": "Louisiana", "ME": "Maine", "MD": "Maryland",
	"MA": "Massachusetts", "MI": "Michigan", "MN": "Minnesota", "MS": "Mississippi",
	"MO": "Missouri", "MT": "Montana", "NE": "Nebraska", "NV": "Nevada",
	"NH": "New Hampshire", "NJ": "New Jersey", "NM": "New Mexico", "NY": "New York",
	"NC": "North Carolina", "ND": "North Dakota", "OH": "Ohio", "OK": "Oklahoma",
	"OR": "Oregon", "PA": "Pennsylvania", "RI": "Rhode Island", "SC": "South Carolina",
	"SD": "South Dakota", "TN": "Tennessee", "TX": "Texas", "UT": "Utah",
	"VT": "Vermont", "VA": "Virginia", "WA": "Washington", "WV": "West Virginia",
	"WI": "Wisconsin", "WY": "Wyoming", "DC": "District of Columbia",
	"PR": "Puerto Rico", "GU": "Guam", "VI": "U.S. Virgin Islands",
}

func (s *Spec) applyDefaults() {
	if s.Timezone == "" {
		s.Timezone = "America/New_York"
	}
	if s.AdminUsername == "" {
		s.AdminUsername = "admin"
	}
	if s.CRMPort == 0 {
		s.CRMPort = 8082
	}
	// A short name is just the full name unless the post gave a friendlier one.
	if strings.TrimSpace(s.PostShortName) == "" {
		s.PostShortName = s.PostName
	}
	// Derive the full state name from the abbreviation when not supplied.
	if strings.TrimSpace(s.RegionLong) == "" {
		if full, ok := usStates[strings.ToUpper(strings.TrimSpace(s.Region))]; ok {
			s.RegionLong = full
		}
	}
	// serviceArea defaults to the locality if unset.
	if strings.TrimSpace(s.ServiceArea) == "" && s.Locality != "" {
		s.ServiceArea = s.Locality
	}
}

// Validate checks the required fields and basic formats. Returns the first
// problem found (with all missing-required collected into one message).
func (s *Spec) Validate() error {
	var missing []string
	// The genuine essentials. postShortName/regionLong/serviceArea are derived
	// in applyDefaults; phone is optional (not every post publishes one).
	req := map[string]string{
		"client":   s.Client,
		"domain":   s.Domain,
		"postName": s.PostName,
		"locality": s.Locality,
		"region":   s.Region,
		"email":    s.Email,
	}
	for k, v := range req {
		if strings.TrimSpace(v) == "" {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("spec missing required field(s): %s", strings.Join(missing, ", "))
	}
	if strings.TrimSpace(s.RegionLong) == "" {
		return fmt.Errorf("region %q is not a recognized US state abbreviation; set regionLong explicitly", s.Region)
	}
	if !slugRE.MatchString(s.Client) {
		return fmt.Errorf("client %q must be a slug (lowercase letters, digits, hyphens)", s.Client)
	}
	if !domainRE.MatchString(s.Domain) {
		return fmt.Errorf("domain %q is not a valid bare domain", s.Domain)
	}
	if s.CRMPort < 1024 || s.CRMPort > 65535 {
		return fmt.Errorf("crmPort %d out of range (1024-65535)", s.CRMPort)
	}
	for i, o := range s.Officers {
		if o.Role == "" || o.Name == "" {
			return fmt.Errorf("officer #%d needs at least role and name", i+1)
		}
	}
	return nil
}
