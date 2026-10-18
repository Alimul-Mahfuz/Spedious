# Project Plan: Spedious v1, an Open-Source API Load & Throttle Tester

> **Name: Spedious** (engine release line: **v1**). CLI command: `spedious` (short alias: `spd`). Go module: `github.com/<owner>/spedious`. Docker image: `<owner>/spedious`. A JMeter/k6-style load testing tool that simulates many virtual users running scripted API workflows, with a CLI-first design and an optional embedded web UI.

---

## 0. Instructions for the Agent

Read this whole document before writing code. Then:

1. Work **phase by phase** in the order given. Do not start a phase until the previous phase's acceptance criteria pass.
2. After each phase: run tests, run `go vet` and `golangci-lint`, update docs, and commit with a conventional commit message (`feat:`, `fix:`, `docs:`, `test:`, `chore:`).
3. Keep the **engine generic**. It must never contain domain concepts such as "order", "checkout" or "cart". It only knows requests, variables, extractors, assertions, timing, and flow control.
4. Prefer the standard library. Add a third-party dependency only when it clearly saves significant work, and note why in the PR or commit description.
5. If a requirement is ambiguous, choose the simplest reasonable option, document the assumption in `docs/decisions/` as a short ADR, and continue.
6. Every public package needs tests. Every user-facing feature needs a documentation entry and an example under `examples/`.

---

## 1. Goals and Non-Goals

### Goals
- Simulate thousands of concurrent virtual users (VUs) on a single machine with low overhead.
- Let users describe **any API workflow** (ordered steps, variable passing, assertions, think time, branching) without writing code.
- Support advanced users with scripting and a Go SDK.
- Provide accurate latency metrics (avoid coordinated omission) with live reporting.
- Be CLI-first, CI/CD friendly, and usable headless. The web UI is a convenience layer.
- Be easy to extend: protocols, importers, and outputs are plugins behind interfaces.
- Be a polished open-source project: permissive license, good docs, Docker image, single binary.

### Non-Goals (for v1)
- Browser-level (real browser) testing.
- Distributed multi-node execution (design for it, do not build it yet).
- Visual drag-and-drop scenario builder (a YAML editor is enough for v1).
- Cloud-hosted service.

---

## 2. Tech Stack

| Concern | Choice | Notes |
|---|---|---|
| Engine language | **Go** (latest stable) | Goroutines for VUs, single static binary |
| CLI | `cobra` | Subcommands: `run`, `serve`, `import`, `validate`, `version` |
| Config format | YAML (and JSON) | Parsed into one internal model |
| Scripting (Phase 6) | `goja` (embedded JavaScript) | Same approach as k6 |
| Latency histograms | `HdrHistogram` Go port | Do not store every sample |
| JSONPath | a maintained Go library | Used by extractors and assertions |
| HTTP | `net/http` + `httptrace` | Timing breakdown: DNS, connect, TLS, TTFB, total |
| API | REST + Server-Sent Events | Defined by an OpenAPI spec |
| Web UI (Phase 5) | Svelte + Vite + uPlot | Embedded into the binary with `go:embed` |
| Testing | `testing`, `testify`, `httptest` | Plus a mock target server for integration tests |
| Lint/CI | `golangci-lint`, GitHub Actions | Build, test, lint, release |
| Release | `goreleaser` + Docker | Linux, macOS, Windows binaries |
| License | Apache-2.0 | Add `LICENSE` and `NOTICE` |

---

## 3. Architecture

```
UI editor / importers  ──┐
YAML / JSON config     ──┼──►  Internal scenario model  ──►  Engine
JS script (later)      ──┤       (steps, vars, checks)        │
Go SDK                 ──┘                                    ▼
                                                      Protocol plugins (HTTP, ...)
                                                              │
                                                              ▼
                                                  Metrics collector ──► Outputs
                                                  (histograms)         (console, JSON,
                                                                        SSE/UI, Prometheus)
```

**Key rule:** all input formats compile down to a single internal scenario model. The engine only understands that model.

### Core concepts

- **Scenario**: a named workflow, an ordered list of steps plus a load profile.
- **Step**: one unit of work. In v1 a step is a protocol request, a sleep, a group, a condition, or a loop.
- **Virtual User (VU)**: one goroutine running a scenario in a loop or once, with its own isolated context (variables, cookies, HTTP client state).
- **Executor / load shape**: controls how VUs are started (constant users, ramping users, constant arrival rate, ramping arrival rate, per-VU iterations, shared iterations).
- **Protocol**: pluggable interface that executes a step. HTTP first.
- **Extractor**: pulls values from a response into VU variables (JSONPath, regex, header, status).
- **Assertion / check**: validates a response (status, body, JSONPath, header, latency threshold).
- **Threshold**: pass/fail criteria for the whole test (for example `p95 < 500ms`, `error_rate < 1%`). Determines the CLI exit code.
- **Feeder / data source**: supplies per-VU test data (CSV, JSON, generator) with strategies: sequential, random, unique-per-VU.
- **Fragment**: a reusable named block of steps (for example `login`) that scenarios can include.
- **Sync point (barrier)**: optional rendezvous where N VUs wait, then release together. Must have a timeout.

### Concurrency models

Support **both**:
- **Closed model**: fixed number of VUs; each starts a new iteration when the previous finishes.
- **Open model**: iterations start at a target arrival rate regardless of response time. This is required to avoid coordinated omission. When the system cannot keep up, record **dropped iterations** and report them.

### Latency correctness
- Record latency from the **intended** send time in open-model executors.
- Store results in HDR histograms, per metric name and per tag set.
- Provide p50, p90, p95, p99, p99.9, min, max, mean, and count.

---

## 4. Proposed Repository Layout

```
spedious/
├── cmd/spedious/            # main.go, cobra command wiring only
├── internal/
│   ├── scenario/            # internal model, validation, YAML/JSON loader
│   ├── engine/              # runner, VU lifecycle, executors
│   ├── protocol/            # Protocol interface
│   │   └── http/            # HTTP implementation
│   ├── vars/                # templating, built-in functions, per-VU store
│   ├── extract/             # JSONPath, regex, header extractors
│   ├── check/               # assertions and thresholds
│   ├── feed/                # CSV/JSON/generator data feeders
│   ├── metrics/             # collector, histograms, aggregation
│   ├── output/              # console, JSON, SSE, Prometheus writers
│   ├── sync/                # barriers with timeouts
│   ├── importer/            # curl, OpenAPI, Postman, HAR
│   ├── script/              # goja integration (Phase 6)
│   └── server/              # REST + SSE API, embedded UI
├── pkg/loadtest/            # public Go SDK (stable API surface)
├── web/                     # Svelte UI source (Phase 5)
├── api/openapi.yaml         # API spec (source of truth)
├── examples/                # runnable scenarios
├── testdata/                # fixtures
├── docs/                    # user docs, ADRs in docs/decisions/
├── .github/workflows/       # CI, release
├── Dockerfile
├── Makefile
├── LICENSE
├── README.md
├── CONTRIBUTING.md
├── CODE_OF_CONDUCT.md
└── plan.md
```

---

## 5. Scenario File Format (v1)

This is the user-facing contract. Keep it stable and versioned (`version: 1`).

```yaml
version: 1
name: example_api_test

variables:
  baseUrl: https://api.example.com

data:
  users:
    type: csv
    file: ./users.csv
    strategy: unique        # unique | sequential | random

fragments:
  login:
    - request:
        name: login
        method: POST
        url: ${baseUrl}/auth/login
        body:
          email: ${data.users.email}
          password: ${data.users.password}
      extract:
        token: { jsonpath: $.token }
      assert:
        - status == 200

scenarios:
  main_flow:
    weight: 70                         # share of VUs running this flow
    load:
      executor: ramping-users          # see executors below
      stages:
        - { duration: 1m, target: 100 }
        - { duration: 5m, target: 100 }
        - { duration: 30s, target: 0 }
    steps:
      - include: login
      - request:
          name: list_items
          method: GET
          url: ${baseUrl}/items
          headers: { Authorization: "Bearer ${token}" }
        extract:
          itemId: { jsonpath: $[0].id }
        thinkTime: 1s-3s
      - sync:                          # optional barrier
          name: all_ready
          waitFor: 100
          timeout: 30s
      - request:
          name: create_thing
          method: POST
          url: ${baseUrl}/things
          headers:
            Authorization: "Bearer ${token}"
            Idempotency-Key: ${uuid()}
          body: { itemId: "${itemId}" }
        assert:
          - status in [200, 201, 409]
          - latency < 800ms
      - if:
          condition: ${status} == 409
          then:
            - sleep: 500ms

thresholds:
  - metric: http_req_duration
    rule: p(95) < 500ms
  - metric: error_rate
    rule: rate < 0.01
```

### Required v1 features of the format
- `variables`, templating with `${...}`, and built-in functions: `uuid()`, `random(min,max)`, `randomString(n)`, `now()`, `env("NAME")`, `timestamp()`.
- Request fields: method, url, headers, query, body (JSON, form, raw, file), cookies, timeout, auth helpers (basic, bearer).
- Extractors: JSONPath, regex, header, status, plus response time.
- Assertions: status, status-in-list, body contains, JSONPath comparison, header, latency.
- Failure policy per step: `onFailure: abort_iteration | continue | abort_test`.
- Expected business outcomes: allow users to classify certain statuses as expected (for example 409 or 404) so they are not counted as errors.
- Think time: fixed or random range.
- Control flow: `if`, `loop` (count and while), `group` (named transaction measured end to end), `include` (fragments).
- Weighted scenario mix.
- Thresholds with CLI exit code.

### Executors
- `constant-users`
- `ramping-users`
- `per-vu-iterations`
- `shared-iterations`
- `constant-arrival-rate` (open model)
- `ramping-arrival-rate` (open model)

---

## 6. Phased Roadmap

### Phase 0: Project Setup
**Tasks**
- Initialize Go module, repo layout, `Makefile` (`build`, `test`, `lint`, `run`, `release`).
- Add LICENSE (Apache-2.0), README stub, CONTRIBUTING, CODE_OF_CONDUCT.
- Add GitHub Actions: build and test on Linux/macOS/Windows, lint, race detector (`go test -race`).
- Add a **mock target server** (`testdata/mockserver`) with configurable latency, error rates, auth, and stateful endpoints for integration tests.

**Acceptance**
- `make build test lint` passes on a clean checkout. CI is green.

---

### Phase 1: Core Engine and HTTP (MVP)
**Tasks**
- Define the internal scenario model in `internal/scenario` with validation and clear, line-numbered error messages for bad YAML.
- Implement the `Protocol` interface and the HTTP protocol with `httptrace` timing breakdown.
- Implement the per-VU context: variable store, cookie jar, own HTTP client state.
- Implement templating and built-in functions.
- Implement extractors (JSONPath, regex, header, status).
- Implement assertions and failure policies.
- Implement think time, `sleep`, `if`, `loop`, `group`, `include`.
- Implement `constant-users` and `per-vu-iterations` executors.
- Implement graceful shutdown (context cancellation, SIGINT) and per-request timeouts.
- CLI: `spedious run scenario.yaml` and `spedious validate scenario.yaml`.
- Console output: live progress line and end-of-test summary.

**Acceptance**
- Running the example scenario against the mock server executes steps in order per VU, passes variables between steps, and isolates state between VUs (verified by test).
- 1,000 concurrent VUs run against the mock server with stable memory (document measured numbers).
- No data races under `go test -race`.

---

### Phase 2: Metrics, Load Shapes, and Thresholds
**Tasks**
- Build the metrics collector: counters, rates, trends (HDR histograms), tagged by scenario, step name, status, and custom tags.
- Built-in metrics: `http_reqs`, `http_req_duration`, `http_req_blocked`, `http_req_connecting`, `http_req_tls_handshaking`, `http_req_waiting`, `http_req_receiving`, `iterations`, `iteration_duration`, `vus`, `error_rate`, `dropped_iterations`, `data_sent`, `data_received`.
- Implement `ramping-users`, `shared-iterations`, `constant-arrival-rate`, and `ramping-arrival-rate`.
- Correct latency accounting for open-model executors (measure from intended start).
- Thresholds with pass/fail evaluation and non-zero exit code on failure.
- Weighted scenario mix.
- Outputs: console summary, JSON summary file, CSV time series.
- Optional abort-on-threshold-fail.

**Acceptance**
- A test against a mock server with an artificial stall shows correct (not understated) p99 in open-model mode (add a regression test for coordinated omission).
- Thresholds control the process exit code.
- Dropped iterations are reported when the target rate cannot be reached.

---

### Phase 3: Data Feeders, Sync, and Generality
**Tasks**
- Data feeders: CSV, JSON, inline list; strategies `unique`, `sequential`, `random`. Define behavior on exhaustion (`stop`, `wrap`, `fail`).
- Barriers (`sync`) with count, timeout, and release policy. Must never hang the test.
- Reusable fragments and parameterized includes.
- Environment variable and CLI overrides (`--var key=value`, `--env-file`).
- Request bodies: form, multipart, file upload, raw, gzip.
- Auth helpers and header presets.

**Acceptance**
- 500 VUs each log in with a unique CSV account (verified on the mock server).
- A barrier releases N VUs within a small, measured window, and times out cleanly when fewer than N arrive.

---

### Phase 4: Importers and API Server
**Tasks**
- Importers generating scenario YAML: `cURL`, `OpenAPI 3`, `Postman v2.1`, `HAR`. CLI: `spedious import <type> <file> -o scenario.yaml`.
- Write `api/openapi.yaml` first, then implement the server from it.
- REST endpoints: create/validate scenario, start/stop/pause a run, list runs, get run summary, get run results, health.
- Live metrics stream via Server-Sent Events.
- Persist run history (start with local JSON files or SQLite; hide behind an interface).
- CLI: `spedious serve --addr :8080`.
- Prometheus metrics output endpoint.

**Acceptance**
- A run can be started, observed live over SSE, stopped, and fetched later purely through the API.
- Importer outputs validate and run against the mock server (golden-file tests).

---

### Phase 5: Web UI
**Tasks**
- Svelte + Vite app in `web/`, built and embedded with `go:embed` so `spedious serve` needs no extra install.
- Pages: Scenarios (list and edit), Run (configure and start), Live Dashboard, Results (history and detail), Compare Runs.
- Scenario editor: Monaco with YAML validation against a JSON Schema generated from the scenario model, plus a "validate" button calling the API.
- Live dashboard (uPlot): RPS, latency percentiles, error rate, active VUs, dropped iterations; per-step table.
- Export run report as HTML and JSON.
- Responsive layout, light and dark theme, keyboard accessible.

**Acceptance**
- From a fresh binary: `spedious serve`, open the UI, paste a scenario, run it, watch live charts, and view the saved result.
- Frontend lint and unit tests pass in CI. The production bundle is embedded in release builds.

---

### Phase 6: Scripting and Go SDK
**Tasks**
- Embed `goja`. Expose a small, stable JS API: `http.get/post/...`, `check()`, `sleep()`, `group()`, `data`, `env`, `vu.id`, `metrics.add()`.
- Allow scenarios to reference a script (`script: ./flow.js`) while still using YAML for load profile and thresholds.
- Public Go SDK in `pkg/loadtest` for running tests as ordinary Go code.
- Document the stability policy for `pkg/` (semver).

**Acceptance**
- The same workflow works as YAML, as JS, and as Go SDK code with equivalent results.
- Script errors report file and line.

---

### Phase 7: Extensibility and Additional Protocols
**Tasks**
- Finalize and document the `Protocol` interface. Add a `docs/extending.md` guide.
- Implement WebSocket and gRPC protocols as proof the abstraction is generic.
- Output plugin interface (InfluxDB, OpenTelemetry).
- Optional: distributed mode design doc (controller and workers), no implementation yet.

**Acceptance**
- A new protocol can be added without modifying `internal/engine`.

---

### Phase 8: Release and Community
**Tasks**
- `goreleaser` config, Docker image (multi-arch), Homebrew formula (optional).
- Documentation site (or `docs/` rendered via GitHub Pages): quick start, scenario reference, executors, metrics, thresholds, examples, FAQ, comparison with JMeter/k6/Locust.
- Issue and PR templates, security policy (`SECURITY.md`), changelog.
- Benchmark doc: VUs and RPS per CPU/RAM on reference hardware.
- Tag `v0.1.0` for the first public preview. Tag `v1.0.0` once the scenario format, CLI flags and `pkg/` SDK are declared stable (see section 7, Compatibility).

**Acceptance**
- A new user can install, run an example, and see results in under 2 minutes following the README.

---

## 7. Quality Requirements

- **Testing**: unit tests for every package; integration tests against the mock server; `-race` in CI; target at least 80% coverage on `internal/engine`, `scenario`, `vars`, `extract`, `check`, and `metrics`.
- **Performance**: avoid per-request allocations in hot paths; reuse HTTP transports; configurable connection limits; benchmark critical paths (`go test -bench`).
- **Reliability**: no goroutine leaks (verify with `goleak`); all waits are context-aware and time-bounded.
- **Safety**: add a clear notice in docs and CLI that the tool must only be used against systems the user owns or is authorized to test. Provide a global rate cap option (`--max-rps`) as a guard against accidental overload.
- **Errors**: actionable messages with file, line, and field name for config problems.
- **Security**: redact secrets (tokens, passwords, auth headers) in logs and reports by default; never log request bodies unless asked.
- **Compatibility**: scenario format is versioned; breaking changes need a new `version`. Follow semver for the CLI and `pkg/`. Note: a Go module at v1 uses the plain path (`github.com/<owner>/spedious`); only v2 and later add a `/v2` suffix.

---

## 8. Definition of Done (per feature)

- [ ] Code merged with tests passing (including `-race`)
- [ ] Lint clean
- [ ] Documentation updated
- [ ] Example added under `examples/`
- [ ] Changelog entry added
- [ ] No domain-specific logic added to the engine

---

## 9. Suggested First Tasks for the Agent

1. Complete Phase 0 and commit.
2. Write ADR-001 (`docs/decisions/001-architecture.md`) summarizing section 3 in your own words, and list any deviations you plan to make.
3. Define the internal scenario model as Go structs with validation and write table-driven tests for the loader.
4. Build the mock target server.
5. Implement the simplest end-to-end path: load YAML, run 10 VUs, send one GET request, print a summary. Then iterate toward the Phase 1 acceptance criteria.

---

## 10. Open Questions (agent should propose defaults, not block)

- Replace `<owner>` in the module path and Docker image name with the real GitHub owner/org.
- Run history storage: JSON files vs SQLite.
- JSONPath library choice (evaluate correctness and performance).
- Whether the UI should support editing scenarios from imported OpenAPI specs inline in v1.
