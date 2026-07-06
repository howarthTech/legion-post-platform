// billing — platform billing against Square.
//
//	billing -secrets <square.env> setup
//	    Idempotently create the two subscription plans in the Square catalog
//	    (Website $149/yr, SMS CRM add-on $50/yr) and print their IDs.
//
//	billing -secrets <square.env> subscribe -spec clients/<name>.yaml [-plans website,sms]
//	    Ensure a Square customer for the client and print a checkout link per
//	    plan. Send the link(s) to the post; paying one starts the annual
//	    subscription with the card on file.
//
//	billing -secrets <square.env> status
//	    List every subscription on the account with status and renewal date.
//
// The secrets file is key=value (SQUARE_ENV, SQUARE_ACCESS_TOKEN,
// SQUARE_LOCATION_ID); process env vars override. Defaults to sandbox unless
// SQUARE_ENV=production.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/howarthTech/legion-post-platform/internal/billing"
	"github.com/howarthTech/legion-post-platform/internal/spec"
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
		return fmt.Errorf("usage: billing -secrets square.env <setup|subscribe|status> [args]")
	}
	cmd, args := flag.Arg(0), flag.Args()[1:]

	c, err := billing.NewClientFromEnvFile(*secretsPath)
	if err != nil {
		return err
	}

	switch cmd {
	case "setup":
		return runSetup(c)
	case "subscribe":
		return runSubscribe(c, args)
	case "status":
		return runStatus(c)
	default:
		return fmt.Errorf("unknown command %q (want setup, subscribe, or status)", cmd)
	}
}

func runSetup(c *billing.Client) error {
	plans, err := c.EnsurePlans()
	if err != nil {
		return err
	}
	fmt.Println("Square catalog ready:")
	for _, p := range plans {
		fmt.Printf("  %-8s %s — $%d.%02d/yr\n           plan %s\n           variation %s\n",
			p.Key, p.Name, p.PriceCents/100, p.PriceCents%100, p.PlanID, p.VariationID)
	}
	return nil
}

func runSubscribe(c *billing.Client, args []string) error {
	fs := flag.NewFlagSet("subscribe", flag.ExitOnError)
	specPath := fs.String("spec", "", "client spec YAML (clients/<name>.yaml)")
	planList := fs.String("plans", "website", "comma-separated plan keys: website,sms")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *specPath == "" {
		return fmt.Errorf("subscribe: -spec is required")
	}
	s, err := spec.Load(*specPath)
	if err != nil {
		return err
	}

	plans, err := c.EnsurePlans()
	if err != nil {
		return err
	}
	byKey := map[string]billing.EnsuredPlan{}
	for _, p := range plans {
		byKey[p.Key] = p
	}

	customerID, err := c.EnsureCustomer(s)
	if err != nil {
		return err
	}
	fmt.Printf("Customer for %s (%s): %s\n\n", s.PostName, s.Client, customerID)

	for _, key := range strings.Split(*planList, ",") {
		key = strings.TrimSpace(key)
		p, ok := byKey[key]
		if !ok {
			return fmt.Errorf("unknown plan key %q (have: website, sms)", key)
		}
		link, err := c.CreateCheckoutLink(s, p)
		if err != nil {
			return err
		}
		fmt.Printf("  %s — $%d.%02d/yr\n  send this link to the post:\n  %s\n\n",
			p.Name, p.PriceCents/100, p.PriceCents%100, link.URL)
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
