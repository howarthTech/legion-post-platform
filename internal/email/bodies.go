package email

import (
	"fmt"
	"html"
	"strings"
)

// Dollars renders cents as a whole-dollar string ("$149").
func Dollars(cents int64) string { return fmt.Sprintf("$%d", cents/100) }

// layout wraps content in a simple, brand-styled, email-client-safe shell.
func layout(heading, inner string) string {
	return `<!DOCTYPE html><html><body style="margin:0;background:#fbf7ec;font-family:Georgia,'Times New Roman',serif;color:#0f1b31">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:#fbf7ec"><tr><td align="center" style="padding:24px 12px">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="max-width:560px;background:#ffffff;border:1px solid #e3ddcf">
  <tr><td style="background:#1a2c50;padding:16px 24px;border-bottom:3px solid #c9a445">
    <span style="color:#e3bf5f;font-size:18px">&#9733;</span>
    <span style="color:#f5efdf;font-family:Arial,Helvetica,sans-serif;font-weight:bold;letter-spacing:1px;text-transform:uppercase;font-size:16px">&nbsp;Legion Post Websites</span>
  </td></tr>
  <tr><td style="padding:28px 24px">
    <h1 style="margin:0 0 16px;font-family:Arial,Helvetica,sans-serif;text-transform:uppercase;color:#1a2c50;font-size:22px">` + heading + `</h1>
    ` + inner + `
  </td></tr>
  <tr><td style="padding:16px 24px;background:#0f1b31;color:rgba(245,239,223,0.7);font-size:12px;font-family:Arial,Helvetica,sans-serif">
    Legion Post Websites — a Howarth Tech Solutions product.
    Not affiliated with, endorsed by, or sponsored by The American Legion.
    Questions? <a href="mailto:support@legionpostwebsites.com" style="color:#e3bf5f">support@legionpostwebsites.com</a>
  </td></tr>
</table></td></tr></table></body></html>`
}

func p(s string) string {
	return `<p style="margin:0 0 14px;font-size:16px;line-height:1.55">` + s + `</p>`
}

// ReceiptHTML is the card-payment receipt sent to the buyer.
func ReceiptHTML(postName, who, planSummary string, totalCents int64) string {
	inner := p("Thank you"+greetName(who)+" — your subscription for <strong>"+html.EscapeString(postName)+"</strong> is active.") +
		`<table role="presentation" cellpadding="0" cellspacing="0" style="margin:8px 0 18px;font-family:Arial,Helvetica,sans-serif;font-size:15px">
       <tr><td style="padding:4px 24px 4px 0;color:#595d65">Plan</td><td style="padding:4px 0"><strong>` + html.EscapeString(planSummary) + `</strong></td></tr>
       <tr><td style="padding:4px 24px 4px 0;color:#595d65">Billed</td><td style="padding:4px 0"><strong>` + Dollars(totalCents) + `/year</strong> (renews annually)</td></tr>
     </table>` +
		p("We'll build and publish your post's website and email you the moment it's live. There's nothing else you need to do right now.") +
		p("You can cancel anytime by replying to this email.")
	return layout("Payment received", inner)
}

// CheckInstructionsHTML tells a check-paying signup where to mail their check.
func CheckInstructionsHTML(postName, who string, totalCents int64, payee string, addressLines []string) string {
	var addr strings.Builder
	addr.WriteString(html.EscapeString(payee))
	for _, l := range addressLines {
		addr.WriteString("<br>" + html.EscapeString(l))
	}
	inner := p("Thanks"+greetName(who)+" — we've got the details for <strong>"+html.EscapeString(postName)+"</strong>.") +
		p("To complete your sign-up, please mail a check for <strong>"+Dollars(totalCents)+"</strong>, payable to <strong>"+html.EscapeString(payee)+"</strong>, to:") +
		`<div style="margin:6px 0 18px;padding:14px 18px;background:#fff;border:2px solid #0f1b31;border-left:5px solid #c9a445;font-family:Arial,Helvetica,sans-serif;font-weight:bold;text-transform:uppercase;letter-spacing:0.5px;line-height:1.6">` + addr.String() + `</div>` +
		p("We'll verify your post and take your website live once your check arrives. We'll email you a confirmation at each step.")
	return layout("Almost there — mail your check", inner)
}

// OperatorHTML is the internal alert to the operator when a post signs up.
func OperatorHTML(method, postName, who, buyerEmail, phone, planSummary string, totalCents int64) string {
	action := "Card payment received — build the site and take it live."
	if method == "check" {
		action = "Awaiting a mailed check. Verify the post; take the site live once the check arrives."
	}
	row := func(k, v string) string {
		if v == "" {
			v = "—"
		}
		return `<tr><td style="padding:3px 20px 3px 0;color:#595d65">` + k + `</td><td style="padding:3px 0"><strong>` + html.EscapeString(v) + `</strong></td></tr>`
	}
	inner := p("<strong>"+strings.ToUpper(method)+"</strong> signup.") +
		`<table role="presentation" cellpadding="0" cellspacing="0" style="margin:6px 0 16px;font-family:Arial,Helvetica,sans-serif;font-size:15px">` +
		row("Post", postName) + row("Contact", who) + row("Email", buyerEmail) +
		row("Phone", phone) + row("Plan", planSummary) + row("Amount", Dollars(totalCents)+"/yr") +
		`</table>` + p(action)
	return layout("New signup: "+html.EscapeString(postName), inner)
}

func greetName(who string) string {
	who = strings.TrimSpace(who)
	if who == "" {
		return ""
	}
	// Use the first token as a friendly first name.
	first := who
	if i := strings.IndexByte(who, ' '); i > 0 {
		// If the first token looks like a title, keep the full "Title Last".
		if isTitle(who[:i]) {
			first = who
		} else {
			first = who[:i]
		}
	}
	return ", " + html.EscapeString(first)
}

func isTitle(s string) bool {
	switch strings.ToLower(s) {
	case "commander", "adjutant", "chaplain", "historian", "finance", "senior", "junior", "sergeant-at-arms", "sergeant", "service", "judge":
		return true
	}
	return false
}
