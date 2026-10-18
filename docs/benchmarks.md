# Spedious Benchmarks & Performance Metrics

## Concurrency Benchmark: 1,000 Virtual Users (Phase 1 Acceptance)

- **Test Target**: Local Spedious Mock Server running on loopback HTTP
- **Virtual Users**: 1,000 concurrent goroutine VUs
- **Requests Sent**: 2,000 requests (2 per VU)
- **Duration**: ~276ms - 287ms
- **Throughput**: ~7,000 - 7,240 req/s on a single machine
- **Errors**: 0 (0.00%)
- **Latency Distribution**:
  - `min`: 0.00ms
  - `med`: 52.06ms
  - `p95`: 199.04ms
  - `p99`: 205.18ms
  - `max`: 211.71ms

## High Throughput Benchmark: 20 VUs Constant Load

- **Test Target**: Local Spedious Mock Server
- **Workload**: Multi-step flow: `auth_login` (POST with JSON body) -> extract token -> `list_items` (GET with auth header) -> extract item ID -> `post_thing` (POST with auth header, UUID idempotency key, extracted ID)
- **Duration**: 3.00s
- **Iterations**: 23,604 iterations (7,865.8 iter/s)
- **Total Requests**: 70,820 requests (23,600.1 req/s)
- **Data Transfer**: 1.8 MB sent, 4.7 MB received
- **Latency**:
  - `med`: 0.61ms
  - `avg`: 0.78ms
  - `p95`: 1.86ms
  - `p99`: 2.51ms
