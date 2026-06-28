# Functional Specifications - gork

This document defines the functional scope and runtime options for `gork`.

## Project Scope
`gork` is an asynchronous, high-performance HTTP benchmarking utility built to measure web server throughput and response latency percentiles under heavy concurrency.

## Core Features
1. **Asynchronous HTTP/1.1 Engine:** Maintain persistent concurrent connection pools.
2. **Lock-Free Telemetry:** Capture request counts, success rates, failures, bytes transferred, and connection drops without lock contention.
3. **Space-Efficient Latency Histogram:** Calculate precise durational percentiles (P50, P90, P99) and mean latencies with zero allocations.
4. **Automated Reconnects:** Recalibrate and dial connections asynchronously upon encountering TCP connection resets or `Connection: close` headers.
