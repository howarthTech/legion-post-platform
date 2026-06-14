# legion-post-platform

The provisioning layer of the **Legion Post Platform** — the SaaS that spins up
a complete website + CRM environment for an American Legion post. Given one
YAML spec per client, the `provision` CLI generates everything needed to stand
that post up.

Part of a three-repo system (see
[plan.md](https://github.com/howarthTech/legion-rome/blob/main/plan.md)):

| Repo | Role |
|---|---|
| [legion-post-theme](https://github.com/howarthTech/legion-rome/tree/main/themes/legion-post-theme) | Shared Hugo theme every client site uses |
| [legion-rome-crm](https://github.com/howarthTech/legion-rome-crm) | Shared CRM image, one container per client |
| **legion-post-platform** (this) | Reads a client spec → generates the per-client environment |

## What it generates

From `clients/<name>.yaml`, `provision` writes `out/<name>/`:

```
out/post-5/
├── CHECKLIST.md                     # the residual manual steps
├── site/
│   ├── hugo.toml                    # full instance config from the spec
│   ├── data/officers.yaml           # the roster from the spec
│   └── content/                     # standard page skeleton (stubs w/ TODO)
├── caddy/
│   ├── <domain>.caddy               # public site block (validated)
│   └── admin.<domain>.caddy         # CRM reverse-proxy block
└── crm/
    ├── <name>.env                   # CRM secrets (mode 600): generated admin
    │                                #   hash + session secret + Twilio blanks
    └── docker-compose.snippet.yml   # the CRM container (shared image)
```

The CLI prints the **one-time admin password** (only the bcrypt hash is
written to disk) and points you at the checklist.

## Usage

```bash
make build
./bin/provision -spec clients/post-5.yaml
# → out/post-5/ + a printed admin password + "read CHECKLIST.md"
```

## The client spec

One YAML file fully describes a tenant. See
[`clients/post-5.yaml`](./clients/post-5.yaml) for a complete, working example
(it reproduces the reference instance). Required fields: `client`, `domain`,
`postName`, `postShortName`, `locality`, `region`, `regionLong`, `email`,
`phone`. Everything else has sensible defaults or is optional (brand colors,
map shortlinks, family contacts).

## How it fits the isolated-tenant model

Each client gets its own everything (see
[plan.md §4](https://github.com/howarthTech/legion-rome/blob/main/plan.md)):
own site directory, own CRM container + SQLite volume, own secrets, own domain
+ TLS, own Twilio number. This tool generates the per-client artifacts; it does
**not** itself deploy them — deployment follows the generated `CHECKLIST.md`
and the OPS hosting runbook. The shared theme and shared CRM image are pulled
in at build/deploy time, not copied per client.

## Dogfooding

`clients/post-5.yaml` is the reference instance expressed as a spec. Running
the provisioner against it reproduces Post 5's environment — the Step 5 "if it
can rebuild client #1, it works" test. The generated `site/hugo.toml` +
`data/officers.yaml` build a working site against the shared theme (verified in
development).

## What it does NOT do (yet)

- **Generic shared pages.** Pages whose text is identical across posts
  (flag etiquette, accessibility statement, membership requirements/why-join/
  apply, family program descriptions) are not generated — the checklist says
  to copy them from the reference instance. A future version may template
  these too.
- **Deploy.** No SSH/rsync/DNS automation yet; the checklist enumerates the
  manual steps. A future version may automate the deploy against the VPS.
- **Port allocation.** `crmPort` is specified per client; the tool doesn't yet
  track which ports are taken across clients.

## Build / dev

```bash
make build      # bin/provision
make run SPEC=clients/post-5.yaml
make test
make vet
```

Requires Go 1.23+.

## License

MIT.
