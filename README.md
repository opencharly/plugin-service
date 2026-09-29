# plugin-service

Service state provisioning for OpenCharly — the `service:` typed-step verb.

`service:` probes and provisions a service across init systems. It is a
host-coupled verb on the SDK kit contract (`CheckVerbProvider` + `ProvisionActor`
+ `StepProvider`), so it is **compiled-in only**.

- **CHECK** — probe running/enabled via `supervisorctl`/`systemctl` through the
  live check engine.
- **ACT** (runtime) — render the enable shell.
- **ACT** (build/deploy) — lower into a `ServicePackagedStep` (the host
  materializes the kit descriptor, keeping the load-bearing `Reverse()` in
  package main).

`service:` is deliberately init-agnostic: it renders to supervisord, systemd or
OpenRC alike. A unit file with no cross-init analogue (a `.socket`, `.target`,
`.slice`, or a drop-in) is what the sibling `unit:` verb exists for.

## What it provides

| Capability | Surface |
|---|---|
| `verb:service` | the `service:` typed step — probe a service (`service:`, `running:`/`enabled:`) and render its enablement |

## How to use it

Compose the plugin candy in a box or check bed's `candy:` list:

```yaml
- '@github.com/opencharly/plugin-service/candy/plugin-service:<tag>'
```

Then author the verb in a plan:

```yaml
- check: supervisord is running
  id: service-supervisord
  service: {service: supervisord, running: true}
  context: [runtime]
```

## Layout

- `candy/plugin-service/` — the plugin module: `plugin.go` (the `verb` +
  `NewCheckVerb()`/`NewMeta()`), `failed_units.go`, `schema/service.cue`,
  `params/cue_types_gen.go`, `cmd/serve/main.go`.
- `charly.yml` — the root project manifest (`discover: candy`).
- `.github/workflows/tag-on-merge.yml` — CalVer tag + `CHANGELOG/` on merge.

## Related

- Owning skill: `/charly-core:service` — the service lifecycle surface. This
  candy carries no `skill:` entity of its own; the gap is tracked in
  [opencharly/opencharly#291](https://github.com/opencharly/opencharly/issues/291).
- `/charly-internals:plugin` — the plugin/provider model.
- [`opencharly/charly`](https://github.com/opencharly/charly) — the charly CLI.
