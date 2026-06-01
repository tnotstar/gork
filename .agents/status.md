---
title: Project Status
project: Gork HTTP Benchmarker
status: healthy
last_verified: 2026-06-01
---

# Project Status - Gork

This document tracks the current execution state, verified performance milestones, and future backlog of the Gork benchmarking tool.

---

## 1. Project Health & Verification

* **Status**: 🟢 **Healthy & Fully Operational**
* **Compilation**: Passes compilation without warnings via Makefile (`make build` or `go build -o gork ./cmd/gork`).
* **Unit Tests**: **95.4% statement coverage** for the `htparser` internal package. All table-driven tests pass successfully via Makefile (`make test` or `go test -v ./internal/htparser/...`).
* **Integration Milestone**: Validated on loopback against a local HTTP server using `http://localhost/pixel.gif`:
  * **Throughput**: **66,352.40 requests per second** (at concurrency 10).
  * **Failure Rate**: **0.00%** (200,974 successful requests, 0 failed responses).
  * **Latency**: Minimum latency of **12 microseconds**, Median (P50) of **100 microseconds**, Mean of **100 microseconds**, P99 of **400 microseconds**, and Maximum of **8.798 milliseconds**.

---

## 2. Completed Milestones

* **Migration to `htparser`**: Completely removed the old stateless parser and integrated the new state-machine based `htparser` package in `cmd/gork/main.go`.
* **Stateful Fragmentation Fix**: Modified the `OnTraffic` loop in `cmd/gork/main.go` to remember `headerParsed` and `bodyOffset` states, completely preventing parser crashes/resets when packets arrive in multiple TCP frames.
* **HTTP/1.0 Connection Close Support**: Built support inside `htparser.Parser` to identify HTTP/1.0 connections and signal connection closure. Gork now gracefully terminates and redials sockets, eliminating `broken pipe` and `connection reset` errors.
* **Lock-free Telemetry**: Created an atomic latency histogram providing constant-space O(1) duration mapping and sub-millisecond percentiles.
* **Declarative Makefile Automation**: Formulated a unified project `Makefile` to automate lint/fmt (`make fmt`), building (`make build`), modular hygiene (`make tidy`), testing (`make test`), and benchmarking (`make run`).

---

## 3. Backlog & Future Tasks

| Task | Priority | Description | Status |
| :--- | :--- | :--- | :--- |
| **HTTP POST Payload Support** | 🔵 Medium | Allow benchmarking POST requests by adding `-m` (method) and `-b` (body) CLI flags, sending pre-vectorized payloads. | 📋 Backlog |
| **HTTPS/TLS Benchmarking** | 🔴 High | Integrate secure TCP dialers or custom TLS handshakes to allow benchmarking `https://` target URLs. | 📋 Backlog |
| **Dynamic Connection Scaling** | 🟡 Low | Implement dynamic connection scaling to adjust concurrency levels during execution. | 📋 Backlog |
| **Custom Header Insertion** | 🔵 Medium | Allow custom headers via a `-H` flag (e.g. `-H "Authorization: Bearer token"`) to test authentication layers. | 📋 Backlog |
