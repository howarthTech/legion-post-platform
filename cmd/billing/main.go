// billing — platform billing against Square.
//
//	billing -secrets <square.env> setup
//	    Idempotently create the subscription billing tiers in the Square catalog
//	    (Website $149/yr, Website + SMS $199/yr) and print their IDs. A post is
//	    billed a single subscription per tier, so it's one charge, not one per
//	    add-on. Run once per environment (sandbox, then production).
//
//	billing -secrets <square.env> status
//	    List every subscription on the account with status and renewal date.
//
// The secrets file is key=value (SQUARE_ENV, SQUARE_ACCESS_TOKEN,
// SQUARE_LOCATION_ID); process env vars override. Defaults to sandbox unless
// SQUARE_ENV=production. Self-service checkout runs through cmd/checkout.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/howarthTech/legion-post-platform/internal/billing"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "billing:", err)
		os.Exit(1)
	}
}

func run() error {
	secretsPath := flag.String("secrets", "", "path to square.env (key=value); env vars override")
	flag.Parse()

	if flag.NArg() < 1 {
		return fmt.Errorf("usage: billing -secrets square.env <setup|status>")
	}
	cmd := flag.Arg(0)

	c, err := billing.NewClientFromEnvFile(*secretsPath)
	if err != nil {
		return err
	}

	switch cmd {
	case "setup":
		return runSetup(c)
	case "status":
		return runStatus(c)
	default:
		return fmt.Errorf("unknown command %q (want setup or status)", cmd)
	}
}

func runSetup(c *billing.Client) error {
	tiers, err := c.EnsureTiers()
	if err != nil {
		return err
	}
	fmt.Println("Square billing tiers ready (each is a single subscription = single charge):")
	for _, t := range tiers {
		fmt.Printf("  %-12s %s — $%d.%02d/yr\n               plan %s\n               variation %s\n",
			t.Key, t.Name, t.PriceCents/100, t.PriceCents%100, t.PlanID, t.VariationID)
	}
	return nil
}

func runStatus(c *billing.Client) error {
	subs, err := c.ListSubscriptions()
	if err != nil {
		return err
	}
	if len(subs) == 0 {
		fmt.Println("No subscriptions yet.")
		return nil
	}
	fmt.Printf("%-24s %-10s %-12s %-12s %s\n", "SUBSCRIPTION", "STATUS", "STARTED", "CHARGED-TO", "CUSTOMER")
	for _, s := range subs {
		fmt.Printf("%-24s %-10s %-12s %-12s %s\n", s.ID, s.Status, s.StartDate, s.ChargedTo, s.Customer)
	}
	return nil
}
