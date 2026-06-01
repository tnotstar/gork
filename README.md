# Gork

> [!WARNING]
> **Work In Progress (WIP)**: Gork is currently under active development and is not yet ready for production environments. APIs, configuration, and structures are subject to breaking changes.

Gork is an asynchronous, high-performance HTTP web benchmarking tool written in Go. 

---

## Technical Foundations

### 1. High-Performance Networking via `gnet/v2`
The extreme speed and throughput of Gork are primarily driven by the event-driven, non-blocking [`gnet/v2`](https://github.com/panjf2000/gnet) networking engine. 
Unlike Go's standard netpoll scheduler (which allocates one goroutine per connection, causing high thread-context switching and stack memory overhead under massive concurrency), `gnet` utilizes a clean, loop-multiplexed epoll/kqueue network model (inspired by Netty and libuv). It processes millions of concurrent network events on a small, pinned OS thread pool, acting as the foundational backbone for Gork's high-concurrency capability.

### 2. Zero-Allocation Nginx-derived State Machine
The `htparser` package is a custom Go translation of the classic Joyent C `http_parser` state machine (originally copied by Joyent from the Nginx HTTP parser and used extensively by the popular [`wrk`](https://github.com/wg/wrk) benchmarking tool). 
While modern Node.js has moved to a parser generator (`llhttp`), Gork relies on this classic, ultra-robust C state machine. Optimized with a zero-heap-allocation memory architecture, it copies header and status tokens directly into a recycled `bytebufferpool` buffer, keeping the hot I/O path completely GC-allocation-free.

---

## Features

* **Event-Driven Non-Blocking I/O**: Leverages `gnet/v2` multicore loop multiplexing, handling massive connection counts with a constant thread footprint.
* **Classic C State Machine Parser**: Translates the Nginx/wrk HTTP state machine to Go, ensuring standard compliance and parsing resilience.
* **Lock-Free Telemetry**: Updates success/failure counters and throughput atomically (`sync/atomic`) without mutex contention.
* **$O(1)$ Space Latency Histogram**: Maps durational latencies directly into 2000 linear buckets of 100 microseconds (0 to 200ms range) to calculate precise percentiles (P50, P90, P99) and mean latencies with zero allocations.
* **HTTP/1.0 Connection Close & Keep-Alive Support**: Gracefully tracks `Connection: close` headers and automatically handles standard HTTP/1.0 response streams by closing and redialing sockets asynchronously to maintain target concurrency.

---

## Installation & Build

Ensure you have Go 1.26.3+ installed.

```bash
# Clone the repository
# (Follow standard git clone instructions)

# Clean and build the binary via Makefile
make build

# Run the unit tests to verify the parser
make test
```

---

## Usage & Quickstart

Gork targets are fully automated using the `Makefile` during development:

```bash
# Format source files
make fmt

# Clean dependencies
make tidy

# Run a quick local benchmark test (10 concurrent connections for 3 seconds)
make run

# Clean build outputs
make clean
```

For advanced runs, build the binary and execute it specifying your parameters:

```bash
./gork -url http://localhost/pixel.gif -c 100 -d 5s
```

### CLI Options Reference

| Flag | Default | Description |
| :--- | :--- | :--- |
| `-url` | `http://localhost:80/pixel.gif` | The target HTTP URL to benchmark. |
| `-c` | `10` | Concurrency level (number of active concurrent TCP connections to maintain). |
| `-n` | `0` | Total number of requests to complete. If set to `0`, execution is bound only by the duration limit. |
| `-d` | `10s` | Duration of the benchmark (e.g., `10s`, `1m`, `500ms`). |

Both stop criteria (`-n` and `-d`) are supported. The benchmark will terminate as soon as either condition is met.

---

## Technical Architecture

### 1. Connection Recycling & Stateful Multiplexing
Each active socket maintains a persistent `connContext` that wraps:
* A `buf []byte` slice that dynamically accumulates fragmented TCP packets.
* An instance of `htparser.Parser` recycled via `Reset()`.
* Flag states `headerParsed` and `bodyOffset` that isolate header parsing from body streaming, completely eliminating re-parsing overhead when response payloads arrive in chunks.

### 2. Lock-free Atomic Telemetry
All request counters, throughput sizes, and latency boundaries (`minLatency` and `maxLatency`) are updated concurrently by event loop goroutines using atomic lock-free CAS (Compare-And-Swap) loops, preventing execution bottlenecks across CPU cores.

### 3. Latency Histogram Allocation
Latencies are mapped directly to histogram buckets in constant time:
$$\text{bucketIndex} = \frac{\text{latency}}{100\ \mu\text{s}}$$
This linear resolution permits precise percentile computations without retaining individual request times in growing arrays, maintaining zero allocation overhead.

---

## License

This project is distributed under the **Apache License 2.0** - see the `LICENSE` file for full redistribution, patent grant, and liability details.
