package billing

import (
	"fmt"
	"strings"
)

// Plans are the itemized offerings shown on the checkout page (the line items
// the buyer sees). Prices decided 2026-07-06; annual cadence.
var Plans = []PlanDef{
	{Key: "website", Name: "Legion Post Website", PriceCents: 14900},
	{Key: "sms", Name: "Legion Post SMS CRM Add-on", PriceCents: 5000},
}

type PlanDef struct {
	Key        string // stable slug used in the API + selection
	Name       string // display name
	PriceCents int64  // annual price, USD cents
}

// Tiers are what a post is actually BILLED. Each tier is one Square
// subscription, so a post pays a SINGLE combined charge rather than one charge
// per add-on. The checkout page still shows the itemized Plans breakdown; only
// the charge combines. Add a tier for each purchasable combination.
var Tiers = []TierDef{
	{Key: "website", Name: "Legion Post Website", PriceCents: 14900, Includes: []string{"website"}},
	{Key: "website-sms", Name: "Legion Post Website + SMS Reminders", PriceCents: 19900, Includes: []string{"website", "sms"}},
}

type TierDef struct {
	Key        string
	Name       string
	PriceCents int64
	Includes   []string // the display-plan keys this tier bundles
}

// EnsuredTier is a TierDef resolved to live Square catalog IDs.
type EnsuredTier struct {
	TierDef
	PlanID      string
	VariationID string
}

// TierForSelection maps the buyer's selected display plans to the single
// billing tier. "website" is required; selecting "sms" upgrades to the bundle.
func TierForSelection(planKeys []string) *TierDef {
	sms := false
	for _, k := range planKeys {
		if strings.EqualFold(strings.TrimSpace(k), "sms") {
			sms = true
		}
	}
	key := "website"
	if sms {
		key = "website-sms"
	}
	for i := range Tiers {
		if Tiers[i].Key == key {
			return &Tiers[i]
		}
	}
	return nil
}

type catalogObject struct {
	Type                 string          `json:"type"`
	ID                   string          `json:"id"`
	SubscriptionPlanData *subPlanData    `json:"subscription_plan_data,omitempty"`
	SubscriptionVarData  *subPlanVarData `json:"subscription_plan_variation_data,omitempty"`
}

type subPlanData struct {
	Name                       string          `json:"name"`
	SubscriptionPlanVariations []catalogObject `json:"subscription_plan_variations,omitempty"`
}

type subPlanVarData struct {
	Name               string  `json:"name"`
	SubscriptionPlanID string  `json:"subscription_plan_id,omitempty"`
	Phases             []phase `json:"phases,omitempty"`
}

type phase struct {
	Cadence string   `json:"cadence"` // ANNUAL
	Ordinal int      `json:"ordinal"`
	Pricing *pricing `json:"pricing,omitempty"`
}

type pricing struct {
	Type       string `json:"type"` // STATIC
	PriceMoney *money `json:"price_money,omitempty"`
}

type money struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

// EnsureTiers makes sure a Square subscription plan (and one annual variation)
// exists for each billing tier, creating whatever is missing. Identity is the
// plan display name — safe to run any number of times.
func (c *Client) EnsureTiers() ([]EnsuredTier, error) {
	existing, err := c.listSubscriptionPlans()
	if err != nil {
		return nil, err
	}

	var out []EnsuredTier
	for _, def := range Tiers {
		et := EnsuredTier{TierDef: def}

		if found, ok := existing[def.Name]; ok {
			et.PlanID = found.ID
			if len(found.SubscriptionPlanData.SubscriptionPlanVariations) > 0 {
				et.VariationID = found.SubscriptionPlanData.SubscriptionPlanVariations[0].ID
			}
		} else {
			var resp struct {
				CatalogObject catalogObject `json:"catalog_object"`
			}
			err := c.do("POST", "/v2/catalog/object", map[string]any{
				"idempotency_key": fmt.Sprintf("legion-tier-%s-v1", def.Key),
				"object": catalogObject{
					Type:                 "SUBSCRIPTION_PLAN",
					ID:                   "#" + def.Key,
					SubscriptionPlanData: &subPlanData{Name: def.Name},
				},
			}, &resp)
			if err != nil {
				return nil, fmt.Errorf("create plan %q: %w", def.Name, err)
			}
			et.PlanID = resp.CatalogObject.ID
		}

		if et.VariationID == "" {
			var resp struct {
				CatalogObject catalogObject `json:"catalog_object"`
			}
			err := c.do("POST", "/v2/catalog/object", map[string]any{
				"idempotency_key": fmt.Sprintf("legion-tiervar-%s-v1", def.Key),
				"object": catalogObject{
					Type: "SUBSCRIPTION_PLAN_VARIATION",
					ID:   "#" + def.Key + "-annual",
					SubscriptionVarData: &subPlanVarData{
						Name:               def.Name + " — Annual",
						SubscriptionPlanID: et.PlanID,
						Phases: []phase{{
							Cadence: "ANNUAL",
							Ordinal: 0,
							Pricing: &pricing{
								Type:       "STATIC",
								PriceMoney: &money{Amount: def.PriceCents, Currency: "USD"},
							},
						}},
					},
				},
			}, &resp)
			if err != nil {
				return nil, fmt.Errorf("create variation for %q: %w", def.Name, err)
			}
			et.VariationID = resp.CatalogObject.ID
		}

		out = append(out, et)
	}
	return out, nil
}

// listSubscriptionPlans returns existing plans keyed by display name.
func (c *Client) listSubscriptionPlans() (map[string]catalogObject, error) {
	plans := map[string]catalogObject{}
	cursor := ""
	for {
		path := "/v2/catalog/list?types=SUBSCRIPTION_PLAN"
		if cursor != "" {
			path += "&cursor=" + cursor
		}
		var resp struct {
			Objects []catalogObject `json:"objects"`
			Cursor  string          `json:"cursor"`
		}
		if err := c.do("GET", path, nil, &resp); err != nil {
			return nil, fmt.Errorf("list plans: %w", err)
		}
		for _, o := range resp.Objects {
			if o.SubscriptionPlanData != nil {
				plans[o.SubscriptionPlanData.Name] = o
			}
		}
		if resp.Cursor == "" {
			return plans, nil
		}
		cursor = resp.Cursor
	}
}
