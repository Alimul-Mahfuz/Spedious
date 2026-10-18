# Spedious

> High-performance, open-source API load & throttle testing engine written in Go.

Spedious is a developer-first, CLI-first API load testing tool that simulates thousands of concurrent virtual users (VUs) running scripted API workflows. It provides accurate latency measurements (avoiding coordinated omission) and allows developers to express complex API flows through declarative YAML scenarios or code.

## Features

- **High Concurrency**: Simulate thousands of virtual users on a single machine with minimal memory and CPU overhead.
- **Accurate Latency Accounting**: Support for both closed and open arrival-rate models, avoiding coordinated omission.
- **Declarative YAML Scenarios**: Express complex workflows, variable extractions, data feeders, assertions, think times, branching, and sync barriers.
- **Rich Extensibility**: Pluggable protocol abstractions, importers (cURL, OpenAPI, Postman, HAR), and metrics exporters (Console, JSON, Prometheus).
- **Embedded Web UI**: Single self-contained binary featuring an optional web dashboard with real-time charts.
- **Developer-Friendly CLI**: Clean terminal progress reporting, threshold evaluations, and CI/CD ready exit codes.

📖 **Looking for a step-by-step tutorial without needing to know Go? Check out the [Comprehensive User Guide](docs/user-guide.md).**

## Quick Start

### Installation

```bash
# Clone the repository
git clone https://github.com/spedious/spedious.git
cd spedious

# Build binary
make build
```

### Validate a Scenario

```bash
./bin/spedious validate examples/quickstart.yaml
```

### Run a Load Test

```bash
./bin/spedious run examples/quickstart.yaml
```

## Scenario Overview

```yaml
version: 1
name: quickstart_api_test

variables:
  baseUrl: http://localhost:8080

scenarios:
  default:
    load:
      executor: constant-users
      vus: 10
      duration: 30s
    steps:
      - request:
          name: health_check
          method: GET
          url: ${baseUrl}/health
        assert:
          - status == 200
          - latency < 100ms
```

## Safety Notice

Spedious is designed for load and performance testing. **Only run tests against systems you own or have explicit authorization to test.** Accidental overload can degrade production services. Spedious provides a `--max-rps` safety flag to prevent unintended request rates.

## License

Spedious is licensed under the Apache 2.0 License. See [LICENSE](LICENSE) and [NOTICE](NOTICE) for details.
