# ADR-001: Spedious Architecture and Core Design Decisions

## Status
Accepted

## Context
Spedious is designed as a developer-friendly, high-throughput, CLI-first API load and throttle testing engine. The primary challenge in API load testing tools is balancing concurrency and memory efficiency while avoiding coordinated omission and maintaining clean abstraction between scenario descriptions, protocol execution, and metrics gathering.

## Architectural Summary

### 1. Unified Internal Scenario Model
All input formats (declarative YAML, JSON, future script runners, and SDK invocations) compile down to a single canonical internal scenario representation (`internal/scenario`). The execution engine only consumes and executes this internal model.

Key architectural concepts:
- **Scenario**: A named workflow composed of ordered steps, metadata, and an execution load profile.
- **Step**: Atomic execution unit (Protocol Request, Sleep/ThinkTime, Group, Conditional/If, Loop, Barrier/Sync).
- **Virtual User (VU)**: An isolated goroutine executing an assigned scenario iteration with an independent context (variable store, cookie jar, connection state).
- **Executors**: Load shape managers implementing closed models (`constant-users`, `ramping-users`, `per-vu-iterations`, `shared-iterations`) and open models (`constant-arrival-rate`, `ramping-arrival-rate`).
- **Protocols**: Extensible interfaces where HTTP is the primary implementation, with `net/http` and `httptrace` breakdown.
- **Extractors & Checks**: Dynamic response inspection (JSONPath, Regex, Headers, Status, Latency) feeding variables or triggering threshold violations.
- **Metrics Collector**: Tagged HDR histograms and counters capturing response times, throughput, error rates, and dropped iterations.

### 2. Concurrency and Coordinated Omission Protection
- In **closed models**, VUs iterate continuously or for a fixed count.
- In **open models**, iterations are scheduled according to a Poisson/arrival rate timer. Latency is measured from the *intended start time* rather than the actual start time when backpressure occurs. If maximum concurrent VUs are exhausted, iterations are counted as *dropped iterations* rather than artificially reporting low latencies.

### 3. Decisions & Planned Deviations

1. **Module Path**: `github.com/spedious/spedious` is chosen as the standard Go module name.
2. **Standard Library Preference**: For HTTP and execution, use standard `net/http` with reusable transports and per-VU cookie jars. For CLI, use `github.com/spf13/cobra`.
3. **JSONPath Library**: We choose `github.com/spyzhov/ajson` or a pure Go JSONPath engine that provides high evaluation speed with low allocation overhead.
4. **HDR Histogram**: Use `github.com/HdrHistogram/hdrhistogram-go` for low-overhead dynamic percentile calculations.
5. **Run History Storage**: A pluggable store interface starting with lightweight local JSON runs directory (`.spedious/runs/<id>.json`), leaving SQLite as a secondary backend without requiring CGO.
6. **No Domain Concepts**: The engine remains strictly domain-agnostic. No business logic (e.g. orders, users, carts) will exist in `internal/`.
