# Spedious User Guide: Writing Scenarios & Operating the CLI

> **Audience**: Testers, QA Engineers, DevOps, and Developers.  
> **Prerequisites**: **None!** You do **not** need to know the Go programming language to use Spedious. Spedious runs as a standalone single binary (like `curl`, `kubectl`, or `git`), and test scenarios are written purely in YAML.

---

## Table of Contents

1. [Introduction](#1-introduction)
2. [Installation & Getting the Binary](#2-installation--getting-the-binary)
3. [Quick Start in 2 Minutes](#3-quick-start-in-2-minutes)
4. [Writing Scenarios (YAML Reference)](#4-writing-scenarios-yaml-reference)
   - [Basic Anatomy](#basic-anatomy)
   - [Dynamic Variables & Built-in Functions](#dynamic-variables--built-in-functions)
   - [Requests (Methods, Headers, Body, Auth)](#requests-methods-headers-body-auth)
   - [Response Extractors](#response-extractors)
   - [Assertions (Validating Responses)](#assertions-validating-responses)
   - [Think Time & Delays](#think-time--delays)
   - [Control Flow (`if`, `loop`, `group`)](#control-flow-if-loop-group)
   - [Reusable Fragments (`include`)](#reusable-fragments-include)
   - [Pass/Fail Thresholds](#passfail-thresholds)
5. [Operating the CLI](#5-operating-the-cli)
   - [Validating Scenarios](#validating-scenarios)
   - [Running Scenarios](#running-scenarios)
   - [Command-line Overrides](#command-line-overrides)
6. [Reading & Understanding Test Results](#6-reading--understanding-test-results)
7. [Full Real-World Tutorial](#7-full-real-world-tutorial)
8. [Common Mistakes & Troubleshooting](#8-common-mistakes--troubleshooting)

---

## 1. Introduction

**Spedious** is a high-performance API load and throttle testing tool. It allows you to simulate hundreds or thousands of concurrent virtual users (VUs) executing complex API workflows (e.g., login $\rightarrow$ extract auth token $\rightarrow$ fetch item $\rightarrow$ create order with dynamic data) without writing any code.

---

## 2. Installation & Getting the Binary

Spedious is packaged as a single standalone executable file. You do not need Go, Python, or Node.js installed.

### Option A: Using the Prebuilt Binary
1. Download the `spedious` (or `spedious.exe` for Windows) binary for your operating system.
2. Place it in a folder of your choice or add it to your system `PATH`.
3. Verify by opening a terminal / command prompt and running:
   ```bash
   spedious version
   ```
   *(Short alias `spd` also works)*.

### Option B: Building from Source (Developers Only)
If you already have Go installed and want to build from source:
```bash
make build
# Binary is generated in ./bin/spedious
```

---

## 3. Quick Start in 2 Minutes

Create a file named `quickstart.yaml`:

```yaml
version: 1
name: my_first_test

variables:
  baseUrl: https://httpbin.org

scenarios:
  ping_service:
    load:
      executor: constant-users
      vus: 10              # 10 virtual users
      duration: 10s        # run for 10 seconds
    steps:
      - request:
          name: health_check
          method: GET
          url: ${baseUrl}/get
        assert:
          - status == 200
          - latency < 800ms
```

### Step 1: Validate Your Scenario
Before sending any traffic, check that your YAML syntax and settings are correct:
```bash
spedious validate quickstart.yaml
```
Output:
```text
✓ Scenario "my_first_test" (v1) is valid
  Scenarios defined: 1
  * ping_service (executor: constant-users, steps: 1)
```

### Step 2: Execute the Test
```bash
spedious run quickstart.yaml
```
You will see live terminal progress and an end-of-test metrics summary!

---

## 4. Writing Scenarios (YAML Reference)

Every scenario file starts with `version: 1` and has a `name`.

### Basic Anatomy

```yaml
version: 1
name: test_name

# Global variables accessible anywhere
variables:
  baseUrl: https://api.mycompany.com

# Reusable blocks of steps
fragments:
  # ...

# Workload configurations
scenarios:
  main_workflow:
    load:
      executor: constant-users   # constant-users or per-vu-iterations
      vus: 50
      duration: 30s
    steps:
      # Ordered list of actions
      - request: { ... }

# Performance thresholds that determine pass/fail
thresholds:
  - metric: http_req_duration
    rule: p(95) < 500ms
```

---

### Dynamic Variables & Built-in Functions

Spedious includes a powerful templating engine. Any `${...}` placeholder is evaluated dynamically at runtime.

| Syntax | Description | Example |
|---|---|---|
| `${varName}` | Value of a declared variable | `${baseUrl}/items` |
| `${uuid()}` | Generates a new UUID v4 | `Idempotency-Key: ${uuid()}` |
| `${random(min, max)}` | Random integer between min and max | `count: ${random(1, 100)}` |
| `${randomString(n)}` | Random alphanumeric string of length `n` | `id: "${randomString(8)}"` |
| `${now()}` | Current ISO 8601 timestamp | `timestamp: "${now()}"` |
| `${timestamp()}` | Current epoch time in milliseconds | `time: ${timestamp()}` |
| `${env("NAME")}` | Reads an environment variable from your OS | `token: "${env("API_TOKEN")}"` |
| `${vu.id}` | ID of current Virtual User (e.g., 1, 2, 3...) | `email: "user_${vu.id}@test.com"` |
| `${vu.iteration}` | Current iteration number for this VU | `test_run_${vu.iteration}` |
| `${status}` | HTTP status code of the preceding request | `${status} == 200` |
| `${latency}` | Duration in ms of the preceding request | `${latency} < 250` |

---

### Requests (Methods, Headers, Body, Auth)

A request step executes an HTTP call:

```yaml
- request:
    name: create_user                 # Descriptive step name
    method: POST                      # GET, POST, PUT, DELETE, PATCH, HEAD, OPTIONS
    url: ${baseUrl}/api/v1/users
    headers:
      Content-Type: application/json
      X-Correlation-Id: ${uuid()}
    query:
      notify: "true"
    body:                             # Automatically JSON-encoded if provided as a map
      email: "user_${vu.id}@example.com"
      role: "member"
      age: ${random(18, 65)}
    timeout: 5s                       # Per-request timeout
    onFailure: abort_iteration        # abort_iteration | continue | abort_test
```

#### Authentication Helpers
Spedious provides built-in authentication helpers:

- **Bearer Token**:
  ```yaml
  auth:
    type: bearer
    token: "${jwtToken}"
  ```
- **Basic Auth**:
  ```yaml
  auth:
    type: basic
    username: "admin"
    password: "${env("ADMIN_PASS")}"
  ```

---

### Response Extractors

Extract values from an HTTP response to use in future steps within the same Virtual User.

```yaml
- request:
    name: login_call
    method: POST
    url: ${baseUrl}/login
  extract:
    # 1. JSONPath (searches response JSON body)
    authToken: { jsonpath: "$.token" }
    firstItemId: { jsonpath: "$[0].id" }
    nestedValue: { jsonpath: "$.user.profile.id" }

    # 2. Regular Expression (captures with regex)
    csrfToken: { regex: 'name="_csrf" value="([^"]+)"' }

    # 3. Response Header
    etagVal: { header: "ETag" }

    # 4. Status Code & Latency
    loginStatus: { status: true }
    loginLatency: { latency: true }
```

Once extracted, you can immediately use `${authToken}` or `${firstItemId}` in all following steps!

---

### Assertions (Validating Responses)

Assertions verify that the API is behaving correctly. If an assertion fails, the request is recorded as an error.

```yaml
assert:
  # Status code checks
  - status == 200
  - status != 500
  - status in [200, 201, 204]

  # Latency checks
  - latency < 500ms
  - latency <= 1s

  # Body checks
  - body contains "success"
  - body not contains "Internal Error"

  # Header checks
  - header.Content-Type contains "json"
  - header.X-Cache == "HIT"

  # JSONPath value checks
  - jsonpath $.success == true
  - jsonpath $.count > 0
  - jsonpath $.user.role == "member"
```

---

### Think Time & Delays

Real users do not hammer APIs instantly without pausing. Use `thinkTime` to simulate real-world user pacing:

```yaml
- request:
    name: browse_catalog
    method: GET
    url: ${baseUrl}/catalog
    thinkTime: 1s-3s       # Uniform random pause between 1 and 3 seconds
```

Or a fixed sleep step:
```yaml
- sleep: 500ms             # Pauses execution for 500 milliseconds
```

---

### Control Flow (`if`, `loop`, `group`)

#### 1. Conditional Execution (`if`)
Run steps only when a condition matches:
```yaml
- if:
    condition: ${status} == 409
    then:
      - sleep: 1s
      - request:
          name: retry_call
          method: GET
          url: ${baseUrl}/status
    else:
      - sleep: 100ms
```

#### 2. Repeating Steps (`loop`)
Loop a specified number of times:
```yaml
- loop:
    count: 3
    steps:
      - request:
          name: poll_job
          method: GET
          url: ${baseUrl}/jobs/${jobId}
      - sleep: 200ms
```

#### 3. Group Transactions (`group`)
Group multiple steps into a measured transaction:
```yaml
- group:
    name: complete_checkout
    steps:
      - request: { name: payment, method: POST, url: "${baseUrl}/pay" }
      - request: { name: receipt, method: GET, url: "${baseUrl}/receipt" }
```

---

### Reusable Fragments (`include`)

Define common step sequences once under `fragments:` and reuse them across scenarios:

```yaml
fragments:
  login:
    - request:
        name: do_login
        method: POST
        url: ${baseUrl}/auth/login
        body: { user: "test", pass: "test" }
      extract:
        token: { jsonpath: "$.token" }

scenarios:
  scenario_one:
    load: { executor: constant-users, vus: 10, duration: 10s }
    steps:
      - include: login
      - request: { name: profile, method: GET, url: "${baseUrl}/me", headers: { Authorization: "Bearer ${token}" } }
```

---

### Pass/Fail Thresholds

Thresholds define the service level objectives (SLOs) of your test. If any threshold is breached, Spedious finishes with **exit code 1**, making it seamless to fail a CI/CD build.

```yaml
thresholds:
  - metric: http_req_duration
    rule: p(95) < 300ms       # 95th percentile latency must be under 300ms
  - metric: http_req_duration
    rule: max < 1s            # Maximum response time must be under 1s
  - metric: error_rate
    rule: rate < 0.01         # Total errors must be under 1% of total requests
```

---

## 5. Operating the CLI

### Validating Scenarios

Always validate your scenario before starting a load test:

```bash
spedious validate my_scenario.yaml
```

If you made a typo or wrote an invalid duration, Spedious provides an exact line number:
```text
Error: validation error in my_scenario.yaml: line 14: [scenarios.main.load.executor] invalid executor "unknown-type"
```

---

### Running Scenarios

```bash
spedious run my_scenario.yaml
```

#### Command-line Overrides
You don't need to edit your YAML file to test different loads. You can override settings right from your terminal:

```bash
# Override the number of concurrent virtual users (VUs)
spedious run my_scenario.yaml --vus 100

# Override the duration
spedious run my_scenario.yaml --duration 5m

# Combine overrides
spedious run my_scenario.yaml --vus 50 --duration 2m

# Export JSON summary report
spedious run my_scenario.yaml --summary-export report.json

# Export time-series metrics to CSV
spedious run my_scenario.yaml --out csv=timeseries.csv

# Abort test immediately if any threshold is violated
spedious run my_scenario.yaml --abort-on-fail

# Quiet mode (disables live ticker for clean CI/CD log output)
spedious run my_scenario.yaml --quiet
```

---

### Load Shape Executors Reference

Spedious supports both **Closed** (concurrency-focused) and **Open** (arrival-rate-focused) models:

#### 1. `constant-users` (Closed Model)
A fixed number of VUs looping continuously for a set duration:
```yaml
load:
  executor: constant-users
  vus: 50
  duration: 1m
```

#### 2. `ramping-users` (Closed Model)
Ramps VUs up and down across defined stages:
```yaml
load:
  executor: ramping-users
  stages:
    - { duration: 30s, target: 50 }   # Ramp up to 50 VUs over 30s
    - { duration: 1m, target: 50 }    # Stay at 50 VUs for 1 minute
    - { duration: 15s, target: 0 }    # Ramp down to 0
```

#### 3. `per-vu-iterations` (Closed Model)
Each VU completes a fixed number of iterations, then stops:
```yaml
load:
  executor: per-vu-iterations
  vus: 10
  iterations: 100                    # Each VU runs 100 times (total 1,000 iterations)
```

#### 4. `shared-iterations` (Closed Model)
A shared total pool of iterations distributed among VUs:
```yaml
load:
  executor: shared-iterations
  vus: 10
  iterations: 500                    # 10 workers consume from 500 total iterations
```

#### 5. `constant-arrival-rate` (Open Model - Avoids Coordinated Omission)
Starts iterations at a constant rate regardless of target response times. If the target server slows down, queued latency is accurately captured rather than understated:
```yaml
load:
  executor: constant-arrival-rate
  rate: 100                          # 100 iterations per timeUnit
  timeUnit: 1s
  duration: 1m
  maxVUs: 200                        # Maximum concurrent worker capacity
```

#### 6. `ramping-arrival-rate` (Open Model)
Ramps the arrival rate across stages:
```yaml
load:
  executor: ramping-arrival-rate
  stages:
    - { duration: 30s, target: 50 }   # Ramp to 50 iterations/sec
    - { duration: 1m, target: 100 }   # Ramp to 100 iterations/sec
  maxVUs: 300
```

---

## 6. Reading & Understanding Test Results

While the test is running, you see a live progress line:
```text
[running] elapsed: 5.00s  | VUs: 20   | reqs: 12050  (2410.0/s) | errs: 0 (0.0%) | p95: 1.8ms
```
- **elapsed**: Time the test has been active.
- **VUs**: Number of concurrently running virtual users.
- **reqs**: Total requests completed and current requests per second (RPS).
- **errs**: Total failed requests and percentage error rate.
- **p95**: 95th percentile latency (95% of requests completed faster than this time).

### End-of-Test Summary Report

```text
  ==================== TEST RESULTS SUMMARY ====================
  Total Duration        : 3.00s
  Iterations            : 23604 (7865.8/s)
  Total Requests        : 70820 (23600.1/s)
  Errors                : 7
  Data Sent             : 1.8 MB
  Data Received         : 4.7 MB

  --- Latency Metrics ---
  http_req_duration.....: min=0.00ms med=0.61ms avg=0.78ms max=31.26ms
  percentiles...........: p90=1.64ms p95=1.86ms p99=2.51ms p99.9=3.55ms

  --- Step Breakdown ---
  * auth_login          : reqs=23614 errs=2   avg=0.8ms p95=1.9ms min=0.0ms max=31.3ms
  * list_items          : reqs=23605 errs=1   avg=0.7ms p95=1.8ms min=0.0ms max=6.0ms
  * post_thing          : reqs=23601 errs=4   avg=0.8ms p95=1.9ms min=0.0ms max=4.7ms

  --- Thresholds ---
  ✓ [PASS] p(95) < 200ms
  ✓ [PASS] rate < 0.01
  ==============================================================
  STATUS: PASSED
```

---

## 7. Full Real-World Tutorial

Here is a realistic scenario file testing an e-commerce API. Save this as `ecommerce_test.yaml`:

```yaml
version: 1
name: ecommerce_load_test

variables:
  baseUrl: http://localhost:8080

fragments:
  user_login:
    - request:
        name: login
        method: POST
        url: ${baseUrl}/auth/login
        body:
          email: "tester_${vu.id}@example.com"
          password: "SecretPassword123"
      extract:
        token: { jsonpath: "$.token" }
      assert:
        - status == 200

scenarios:
  customer_journey:
    load:
      executor: constant-users
      vus: 25
      duration: 30s
    steps:
      # Step 1: Login
      - include: user_login

      # Step 2: Browse catalog
      - request:
          name: get_catalog
          method: GET
          url: ${baseUrl}/items
          headers:
            Authorization: "Bearer ${token}"
        extract:
          chosenItem: { jsonpath: "$[0].id" }
        assert:
          - status == 200
          - latency < 350ms
        thinkTime: 500ms-1s

      # Step 3: Place order
      - request:
          name: submit_order
          method: POST
          url: ${baseUrl}/orders
          headers:
            Authorization: "Bearer ${token}"
            Idempotency-Key: ${uuid()}
          body:
            itemId: "${chosenItem}"
            quantity: ${random(1, 5)}
        assert:
          - status in [200, 201]

thresholds:
  - metric: http_req_duration
    rule: p(95) < 500ms
  - metric: error_rate
    rule: rate < 0.02
```

Run it with:
```bash
spedious run ecommerce_test.yaml
```

---

## 8. Common Mistakes & Troubleshooting

### 1. Quotation in YAML Flow Maps
**Problem**: In YAML, brackets like `[0]` inside unquoted flow braces `{ jsonpath: $[0].id }` cause a YAML syntax error.  
**Solution**: Always put quotes around the JSONPath string:
```yaml
# Correct:
extract:
  itemId: { jsonpath: "$[0].id" }
```

### 2. Target Machine Connection Refused
**Problem**: The live ticker shows `100% errs` immediately.  
**Solution**: Check that your target service (`baseUrl`) is running and reachable from your machine. Test with `curl <url>` or a browser.

### 3. Threshold Fails
**Problem**: `STATUS: FAILED (Thresholds breached)` with exit code 1.  
**Solution**: The test completed, but your API's latency or error rate exceeded the limits you configured under `thresholds:`. Check the Step Breakdown to see which endpoint was slow or failing.
