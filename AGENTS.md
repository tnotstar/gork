# AGENTS.md

## Project Overview

Gork is a high-performance, asynchronous HTTP web benchmarking tool written in Go. It is powered by [`gnet/v2`](https://github.com/panjf2000/gnet) for event-driven networking and a custom, zero-allocation state-machine HTTP response parser in the `htparser` package (translated from the classic Joyent C `http_parser` 2.7.1, which Joyent originally copied from Nginx and was used by `wrk`).

It operates concurrently, maintaining a target level of parallel TCP connections, pipelining requests over keep-alive sockets, and compiling lock-free microsecond-resolution latency metrics.

---

## Agent Context Layer (SOT + ADR + Status)

Gork utilizes an automated **Agent Context Layer** stored in the `.agents/` directory to preserve codebase continuity across multiple AI agent sessions. 

### Core Documents in `.agents/`
1. **[`sot.md`](file:///home/tnotstar/Workspaces/Personal/Gork/.agents/sot.md) (Source of Truth)**: The definitive document detailing active system components, interfaces, and packet flow boundaries.
2. **[`adr.md`](file:///home/tnotstar/Workspaces/Personal/Gork/.agents/adr.md) (Architectural Decision Records)**: Chronological index of historic design decisions (e.g., hexagonal architecture relaxation for hot paths, zero-allocation custom C state-machine translation, lock-free linear telemetry histograms).
3. **[`status.md`](file:///home/tnotstar/Workspaces/Personal/Gork/.agents/status.md) (Project Status)**: Details active project health, test status, verified performance metrics (e.g. 66k RPS loopback milestones), and the current engineering backlog.

### Protocol for Incoming Agents
* **On Startup**: **Read `sot.md` and `adr.md`** first to capture structural rules and strict performance/allocation constraints.
* **During Modifications**: **Respect and align** with active records in `adr.md`. Proposing architectural refactoring that violates active ADR constraints is forbidden.
* **On Session Wrap-up**: If you introduce new features, complete tasks, update backlog items, or change active architectural boundaries, you **must update `status.md` and `sot.md` (if structural changes occurred) accordingly** to preserve continuity.

---

## File Layout

* **`main.go`**: Core event loop orchestrator, command-line interface flags parsing, concurrent dialer, keep-alive pipeline engine, and atomic statistics compiler.
* **`internal/htparser/htparser.go`**: State-machine HTTP response parser. Leverages a pre-allocated header slice, `bytebufferpool` string copies, and implements RFC-compliant HTTP/1.0 socket-close detection.
* **`internal/htparser/htparser_test.go`**: Table-driven unit tests verifying response headers parsing, protocol validation, and buffer limit overflows.
* **`Makefile`**: Declarative automation task executor (gofmt, build, test, run, tidy, and clean).

---

## Architecture & Structural Flow

### 1. Connection Event Context (`connContext` in `main.go`)
Each active `gnet.Conn` attaches a stateful pointer to `connContext` containing:
* `parser *htparser.Parser` — a dedicated response parser recycled via `Reset()`.
* `buf []byte` — persistent byte accumulator storing fragmented incoming frames.
* `requestStart time.Time` — VDSO-based high-resolution request start time.
* `headerParsed bool` — isolates header parsing from chunked/length body checks.
* `bodyOffset int` — remembers body offset inside `buf` to skip header parsing on subsequent data chunks.

### 2. State Machine Parser (`htparser.go`)
* Direct, byte-by-byte switch-case loops matching the states of Joyent C `http_parser`.
* Allocates strings into a pooled `bytebufferpool.ByteBuffer` buffer `p.buf.B`, sharing subrebanas (slices) to achieve **zero heap allocation**.
* Tracks headers in a pre-allocated slice `Headers []Header` grown via `copy` only if required.
* **RFC HTTP/1.0 Close Compliance**: In `finalizeConnectionState()`, if `HTTP/1.0` is parsed and no explicit `Connection: keep-alive` header is found, sets `p.ConnectionClose = true`.
* **Security bounds**: Enforces a strict header size limit of `MaxHeaderSize = 80KB` to prevent OOM denial-of-service vector attacks.

### 3. Asynchronous Benchmarking Flow
1. **Bootstrap**: Parses CLI flags (`-url`, `-c`, `-n`, `-d`) and builds the vectorized static HTTP request byte slice `requestBytes` once to avoid string interpolation allocations in the hot path.
2. **Setup Engine**: Creates a `gnet.Client` with `gnet.WithMulticore(true)` and starts the async event loop.
3. **Dial Pool**: Dials target server concurrently `c` times.
4. **Connection Lifecycle**:
   * **`OnOpen`**: Allocates `connContext` and immediately returns `cl.requestBytes` to initiate pipelining.
   * **`OnTraffic`**:
     1. Appends newly arrived data to `ctx.buf` and discards it from the `gnet` system buffer.
     2. If `!ctx.headerParsed`, invokes `ctx.parser.Parse` to parse headers. If incomplete (`ErrMissingData`), returns `gnet.None` to wait.
     3. Once headers are parsed, verifies body complete conditions:
        * *Chunked*: Scans for terminator `0\r\n\r\n`.
        * *Content-Length*: Verifies `len(ctx.buf) >= bodyOffset + contentLength`.
     4. On response completion: Computes VDSO latency, updates telemetry atomically, resets parser/buffer states, and verifies stop limits.
     5. If stop conditions are met, returns `gnet.Close`.
     6. If `p.ConnectionClose` is true, returns `gnet.Close` to release resources cleanly.
     7. Otherwise, immediately triggers `c.AsyncWrite(cl.requestBytes, nil)` to pipeline the next request.
   * **`OnClose`**: If the connection drops or is terminated by the server, Gork redials a new connection asynchronously via a background goroutine to maintain target concurrency `c` until the benchmark completes.

### 4. Telemetry Statistics Engine
To execute completely lock-free, Gork records performance metrics using Go atomic primitives (`sync/atomic`):
* `successRequests` and `failRequests` track completed transactions.
* `bytesRead` and `bytesWritten` monitor network throughput.
* `minLatency` and `maxLatency` bounds are locked-free updated via Compare-And-Swap (CAS) loops.
* **Latency Profile**: Maps latencies into 2000 linear buckets of 100 microseconds (0 to 200ms range) and an overflow bucket. This enables constant time `O(1)` lock-free atomic profiling to calculate P50, P90, P99 and mean latency during reporting.

---

## Developer Commands

Standard automation targets are provided via `make`:
* **Format Source**: `make fmt` (formats codebase via `go fmt`)
* **Clean and Build**: `make build` (compiles the `./gork` binary)
* **Execute Tests**: `make test` (runs unit tests in `./internal/htparser/...`)
* **Reset Workspace**: `make clean` (deletes the compiled binary)
* **Tidy Modules**: `make tidy` (runs `go mod tidy`)
* **Run Benchmark**: `make run` (compiles and runs benchmark with default parameters)
