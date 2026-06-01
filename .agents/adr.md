---
title: Architectural Decision Records (ADR)
project: Gork HTTP Benchmarker
status: active
---

# Architectural Decision Records (ADR) - Gork

This document indexes the major architectural and design decisions that shape Gork's high-performance benchmarking engine.

---

## ADR-001: Relaxation of Hexagonal Architecture in Favor of Performance

### Context
Hexagonal Architecture (Ports & Adapters) isolates domain policies from infrastructure technologies. However, abstracting network packets and parser buffers behind interfaces in Go results in type escapes to the heap (`heap allocations`) and limits inline capability due to dynamic dispatch overhead.

### Decision
We relaxed the strict Port & Adapter interface requirements for the core hot path. Gork's network callbacks and HTTP parser are concrete structures residing directly in `cmd/gork/main.go` and `internal/htparser` package, allowing Go's compiler to optimize allocations, inline key methods, and execute with absolute minimum stack overhead.

### Consequences
* Network layer (`gnet`) and parser (`htparser`) communicate directly through concrete pointers.
* Heap allocations in the request-response loop are reduced to zero, avoiding GC pauses.
* Porting network loops or changing event libraries now requires direct changes to `main.go` rather than just swapping adapters.

---

## ADR-002: Reimplementation of Joyent's C `http_parser` State-Machine

### Context
Go's standard library `net/http` parser is feature-rich but allocates extensive garbage per request (representing headers as `map[string][]string`, allocating strings for status lines, and using generic readers). For a benchmarker doing >100k rps, standard parsing introduces massive CPU and GC bottlenecks.

### Decision
We translated the classic C Joyent `http_parser` state machine (originally copied by Joyent from Nginx and used by `wrk`) into a Go package called `htparser`. It parses bytes incrementally, utilizing a pre-allocated header slice and copying string tokens into a shared `bytebufferpool.ByteBuffer` memory space.

### Consequences
* Header and status lines slice directly into a continuous, recycled byte buffer pool, removing GC pressure.
* Strict compliance with HTTP/1.x, including connection-close tracking and chunked payloads.
* Added standard HTTP/1.0 fallback handling (closing the socket immediately unless `Connection: keep-alive` is explicitly returned).

---

## ADR-003: Lock-free Latency Histogram Telemetry

### Context
Standard benchmarks require accurate percentile statistics (P50, P90, P99). Storing individual latencies in growing arrays or protecting telemetry counters behind a mutex causes CPU cache invalidation, thread lock contention, and high memory growth.

### Decision
We implemented a lock-free telemetry engine utilizing atomic operations (`sync/atomic`) and a pre-allocated linear histogram. The histogram consists of 2000 linear buckets of 100 microseconds (representing 0 to 200ms range) and an overflow bucket. Latencies are mapped directly into buckets using O(1) division:
$$\text{bucketIndex} = \frac{\text{latency}}{100\ \mu\text{s}}$$

### Consequences
* Telemetry updates have zero heap allocation and zero lock overhead, even at extreme concurrency.
* Percentile calculation (P50, P90, P99) is fast and precise up to 200ms, using constant memory.
* Latencies exceeding 200ms are grouped in the overflow bucket and approximated using the atomic maximum latency.
