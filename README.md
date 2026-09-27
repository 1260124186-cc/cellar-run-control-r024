# Cellar Run Control

Cellar Run Control is a local Go HTTP service for coordinating a small
production cellar. It keeps formula approval, vessel availability, active
fermentation runs, gravity observations, conditioning, completion metrics, and
vessel cleaning in one deterministic process.

This repository is a healthy initialization baseline. It has no intentional
defects. Unit tests cover the tag maintenance, filtering, pagination, and
concurrency surfaces, and the production workflow checks in this repository
remain available for immediate build and behavior verification.

## Requirements

- Go 1.23 or later
- A writable local data directory
- No database, network service, or third-party module

## Build And Run

Build every package:

```bash
go build ./...
```

Start the service:

```bash
go run ./cmd/fermentctl serve
```

The default address is `127.0.0.1:8088`. The default snapshot directory is
`./data`.

Environment variables:

- `CELLAR_CONTROL_ADDR`: HTTP listen address.
- `CELLAR_CONTROL_DATA_DIR`: directory containing `state.json`.

Example:

```bash
CELLAR_CONTROL_ADDR=127.0.0.1:9099 \
CELLAR_CONTROL_DATA_DIR=/tmp/cellar-control \
go run ./cmd/fermentctl serve
```

## Directory Structure

```text
cmd/fermentctl                 service and workflow-check executable
internal/clock                 time boundary
internal/config                environment configuration
internal/domain                entities, validation, transitions, metrics
internal/httpapi               HTTP routes, JSON decoding, errors
internal/service               use-case orchestration
internal/storage               revisioned snapshot persistence
internal/workflowcheck         real HTTP production checks
```

## Public API

Health:

- `GET /healthz`

Formula workflow:

- `POST /v1/formulas`
- `GET /v1/formulas`
- `GET /v1/formulas/{id}`
- `PUT /v1/formulas/{id}/steps`
- `PUT /v1/formulas/{id}/tags`
- `POST /v1/formulas/{id}/approve`
- `POST /v1/formulas/{id}/retire`

Formula tags:

- `GET /v1/tags`
- `POST /v1/tags/rename`

`GET /v1/formulas` supports filters that combine with AND:

- `name`: exact full formula name after whitespace and case folding.
- `style`: case-insensitive substring match on the style label.
- `state`: one of `draft`, `approved`, `retired`.
- `tag`: repeatable; a formula must carry every supplied tag to match.

Pagination uses `page` and `page_size` together, both positive integers with
`page_size` at most 200; supplying one without the other, a non-numeric value,
or an out-of-range window returns `invalid_pagination`. The response carries
`total` (the count of the entire filtered set on the snapshot revision),
`page`, `page_size`, and `revision`. Sorting is by normalized name with the
formula identifier as tie-breaker, so pages are stable. Without pagination
parameters the full filtered set is returned and `page`/`page_size` are zero.

Tags are list metadata. Creating a formula and `PUT /v1/formulas/{id}/tags`
accept up to 25 normalized tags (letters, digits, spaces, hyphens,
underscores; at most 40 characters). Duplicate tags after normalization
return `duplicate_tag`; tag maintenance is allowed in every lifecycle state,
including after approval, and never changes process steps, targets, or run
references — only subsequent query results.

`GET /v1/tags` returns each tag in use with its formula count plus the
snapshot `revision`. `POST /v1/tags/rename` requires `X-Expected-Version`
set to that revision (or the current revision): a rename that lands after a
concurrent write fails with `revision_conflict`, an unknown source tag with
`tag_not_found`, and a target already in use with `duplicate_tag`.

Vessel workflow:

- `POST /v1/vessels`
- `GET /v1/vessels`
- `GET /v1/vessels/{id}`
- `POST /v1/vessels/{id}/finish-cleaning`
- `POST /v1/vessels/{id}/retire`

Run workflow:

- `POST /v1/runs`
- `GET /v1/runs`
- `GET /v1/runs/{id}`
- `POST /v1/runs/{id}/reserve`
- `POST /v1/runs/{id}/start`
- `POST /v1/runs/{id}/observations`
- `POST /v1/runs/{id}/conditioning`
- `POST /v1/runs/{id}/complete`
- `POST /v1/runs/{id}/abort`

Writes accept an optional `X-Expected-Version` header. Errors use a stable
shape:

```json
{
  "error": {
    "code": "revision_conflict",
    "detail": "the aggregate version does not match X-Expected-Version",
    "field": ""
  }
}
```

## Workflow Checks

Run each production workflow check separately:

```bash
go run ./cmd/fermentctl check formula-approval
go run ./cmd/fermentctl check vessel-run-lifecycle
go run ./cmd/fermentctl check observations-completion
go run ./cmd/fermentctl check formula-tags
```

Each check creates a temporary snapshot directory, starts the real HTTP
handler on a loopback listener, issues JSON requests, verifies success and
failure behavior, and removes its temporary data.

## Persistence And Recovery

The service stores a complete revisioned snapshot at
`<data-directory>/state.json`. A write creates a temporary file in the same
directory, commits the encoded bytes durably, closes the handle, and renames
the result over the previous snapshot. The in-memory snapshot is replaced only
after that operation succeeds.

On startup, an existing snapshot is decoded and normalized. Unknown or
conflicting map indexes are not silently invented; invalid JSON or a negative
revision stops the process before HTTP serving begins.

## Repair-Ready Boundaries

The baseline intentionally keeps several independent behavior surfaces visible
for later engineering evaluation:

- formula name normalization and duplicate detection;
- target gravity and process-step range validation;
- aggregate version conflict checks;
- atomic vessel reservation and release;
- run state transition guards;
- observation sequence and time ordering;
- gravity movement and range checks;
- metric calculation and rounding;
- durable snapshot replacement and startup normalization.

These are production mechanisms, not injected defects. A later task can add
automated tests or request a focused repair without changing the public API.

## Test Boundary

Unit tests (`go test ./...`) cover tag normalization and limits, tag
maintenance after approval, list filters, stable pagination totals, HTTP
error mapping, and concurrent tag renames. Broader systematic coverage of
validation limits, state transitions, persistence failures, and numeric edge
cases remains available for a later test task.

## Scope

The service does not control physical equipment, authenticate remote users,
send notifications, expose a browser interface, or replicate data across
processes. It is intended for a trusted local network or an internal adapter.
