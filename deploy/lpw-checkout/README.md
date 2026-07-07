# Deploying the checkout backend (lpw-checkout)

The self-service billing service for legionpostwebsites.com. Static checkout
page (`marketing-site/activate.html`) → this backend → Square (customer + card
on file + recurring annual subscription).

**Do not deploy until the Square sandbox test passes** (see step 4). Deploying
a payment endpoint that hasn't been exercised against Square is how money bugs
ship.

## Prerequisites
- `/srv/secrets/square.env` on the box (mode 600, root), with:
  `SQUARE_ENV`, `SQUARE_APPLICATION_ID`, `SQUARE_ACCESS_TOKEN`, `SQUARE_LOCATION_ID`.
- The two subscription plans created in the Square catalog once:
  `./bin/billing -secrets /srv/secrets/square.env setup`

## 1. Build the image on the box
```bash
cd /srv/builds/legion-post-platform
docker build -f Dockerfile.checkout -t lpw-checkout:latest .
```

## 2. Bring up the container
```bash
mkdir -p /srv/apps/lpw-checkout
cp deploy/lpw-checkout/docker-compose.yml /srv/apps/lpw-checkout/
cd /srv/apps/lpw-checkout && docker compose up -d
curl -s localhost:8200/api/healthz   # -> ok
curl -s localhost:8200/api/config    # -> applicationId/locationId/env/plans
```

## 3. Route /api/* to it in Caddy
Add to the `legionpostwebsites.com` site block, BEFORE the static handler
(reverse-proxy the API path, serve everything else static):

```caddy
legionpostwebsites.com {
    encode zstd gzip
    header { ... existing security headers ... }

    handle /api/* {
        reverse_proxy 127.0.0.1:8200
    }

    handle {
        root * /srv/www/legionpostwebsites.com
        try_files {path} {path}/ {path}.html
        file_server
        handle_errors { @404 expression {http.error.status_code} == 404
                        respond @404 "Not found" 404 }
    }
    log { ... }
}
```
`caddy validate --config /etc/caddy/Caddyfile && systemctl reload caddy`

## 4. Sandbox test (the gate before real use)
With `SQUARE_ENV=sandbox`, open https://legionpostwebsites.com/activate and pay.

**Use the Mastercard test card `5105 1051 0510 5100`** (any future expiry, any
CVV, any ZIP) — NOT the Visa `4111 1111 1111 1111`. This is a known Square
**sandbox** bug: subscriptions created with the Visa sandbox card (and the
`cnon:card-nonce-ok` nonce) are auto-DEACTIVATED within seconds without an
invoice, while the Mastercard works. It does NOT affect production or real
cards. (Verified 2026-07-07: with the Visa nonce the customer + card store fine,
the card charges fine one-time, the plan is valid, the subscription is created
ACTIVE — then Square deactivates it. Mastercard subscriptions stay ACTIVE.)

Then confirm in the Square **sandbox** dashboard that a customer, a card on file,
and an ACTIVE subscription were created. Check `docker logs lpw-checkout` for
the `✓ checkout` line.

## 5. Go production
Swap `/srv/secrets/square.env` to the Production-tab values (`SQUARE_ENV=production`),
re-run `billing setup` against production to create the live plans, then
`docker compose up -d --force-recreate`.

## Port
Loopback `127.0.0.1:8200` (platform service; outside the 8081–8099 CRM-tenant
range). Update sysadmin.md's port table.
