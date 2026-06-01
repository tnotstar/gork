package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/panjf2000/gnet/v2"
	"github.com/tnotstar/gork/internal/htparser"
)

const LatencyBuckets = 2000 // Covers 0 to 200ms with 100-microsecond resolution

type connContext struct {
	parser       *htparser.Parser
	buf          []byte
	requestStart time.Time
	headerParsed bool
	bodyOffset   int
}

type metrics struct {
	completedRequests uint64
	successRequests   uint64
	failRequests      uint64
	bytesRead         uint64
	bytesWritten      uint64
}

type httpClient struct {
	gnet.BuiltinEventEngine

	addr           string
	cli            *gnet.Client
	stopFlag       int32
	reqLimit       uint64
	metrics        metrics
	closeLimitOnce sync.Once
	limitReached   chan struct{}
	requestBytes   []byte

	latencyBuckets [LatencyBuckets + 1]uint64
	minLatency     int64
	maxLatency     int64
}

func (cl *httpClient) recordMetrics(status []byte, latency time.Duration, size uint64) {
	if len(status) > 0 && (status[0] == '2' || status[0] == '3') {
		atomic.AddUint64(&cl.metrics.successRequests, 1)
	} else {
		atomic.AddUint64(&cl.metrics.failRequests, 1)
	}
	atomic.AddUint64(&cl.metrics.bytesRead, size)

	ns := latency.Nanoseconds()
	for {
		currentMin := atomic.LoadInt64(&cl.minLatency)
		if ns >= currentMin && currentMin != 0 {
			break
		}
		if atomic.CompareAndSwapInt64(&cl.minLatency, currentMin, ns) {
			break
		}
	}
	for {
		currentMax := atomic.LoadInt64(&cl.maxLatency)
		if ns <= currentMax {
			break
		}
		if atomic.CompareAndSwapInt64(&cl.maxLatency, currentMax, ns) {
			break
		}
	}

	idx := int(latency / (100 * time.Microsecond))
	if idx < 0 {
		idx = 0
	} else if idx >= LatencyBuckets {
		idx = LatencyBuckets
	}
	atomic.AddUint64(&cl.latencyBuckets[idx], 1)
}

func (cl *httpClient) calculatePercentile(p float64) time.Duration {
	var totalRequests uint64
	for i := 0; i <= LatencyBuckets; i++ {
		totalRequests += atomic.LoadUint64(&cl.latencyBuckets[i])
	}
	if totalRequests == 0 {
		return 0
	}

	targetCount := uint64(float64(totalRequests) * p / 100.0)
	var count uint64
	for i := 0; i <= LatencyBuckets; i++ {
		count += atomic.LoadUint64(&cl.latencyBuckets[i])
		if count >= targetCount {
			if i == LatencyBuckets {
				return time.Duration(atomic.LoadInt64(&cl.maxLatency))
			}
			return time.Duration(i) * 100 * time.Microsecond
		}
	}
	return time.Duration(atomic.LoadInt64(&cl.maxLatency))
}

func (cl *httpClient) meanLatency() time.Duration {
	var totalRequests uint64
	var sum int64
	for i := 0; i < LatencyBuckets; i++ {
		c := atomic.LoadUint64(&cl.latencyBuckets[i])
		totalRequests += c
		sum += int64(c) * int64(i) * 100 * int64(time.Microsecond)
	}
	overflowCount := atomic.LoadUint64(&cl.latencyBuckets[LatencyBuckets])
	if overflowCount > 0 {
		totalRequests += overflowCount
		sum += int64(overflowCount) * atomic.LoadInt64(&cl.maxLatency)
	}

	if totalRequests == 0 {
		return 0
	}
	return time.Duration(sum / int64(totalRequests))
}

func (cl *httpClient) OnBoot(eng gnet.Engine) gnet.Action {
	return gnet.None
}

func (cl *httpClient) OnOpen(c gnet.Conn) ([]byte, gnet.Action) {
	ctx := &connContext{
		parser:       htparser.NewParser(),
		requestStart: time.Now(),
	}
	c.SetContext(ctx)

	atomic.AddUint64(&cl.metrics.bytesWritten, uint64(len(cl.requestBytes)))
	return cl.requestBytes, gnet.None
}

func (cl *httpClient) OnClose(c gnet.Conn, err error) gnet.Action {
	ctx, ok := c.Context().(*connContext)
	if ok && ctx != nil {
		if ctx.parser.Status != nil {
			latency := time.Since(ctx.requestStart)
			cl.recordMetrics(ctx.parser.Status, latency, uint64(len(ctx.buf)))
			atomic.AddUint64(&cl.metrics.completedRequests, 1)
		}
		ctx.parser.Release()
	}

	if atomic.LoadInt32(&cl.stopFlag) == 0 {
		go func() {
			_, _ = cl.cli.Dial("tcp", cl.addr)
		}()
	}
	return gnet.None
}

func (cl *httpClient) OnTraffic(c gnet.Conn) gnet.Action {
	ctx := c.Context().(*connContext)
	buf, _ := c.Peek(-1)

	ctx.buf = append(ctx.buf, buf...)
	c.Discard(len(buf))

	var bodyOffset int
	if !ctx.headerParsed {
		n, err := ctx.parser.Parse(ctx.buf)
		if err != nil {
			if errors.Is(err, htparser.ErrMissingData) {
				return gnet.None
			}
			atomic.AddUint64(&cl.metrics.failRequests, 1)
			return gnet.Close
		}
		ctx.headerParsed = true
		ctx.bodyOffset = n
		bodyOffset = n
	} else {
		bodyOffset = ctx.bodyOffset
	}

	contentLength := ctx.parser.ContentLength()

	if ctx.parser.Chunked {
		idx := bytes.Index(ctx.buf[bodyOffset:], []byte("0\r\n\r\n"))
		if idx == -1 {
			return gnet.None
		}
		bodyEnd := bodyOffset + idx + 5
		if len(ctx.buf) < bodyEnd {
			return gnet.None
		}
		ctx.buf = ctx.buf[bodyEnd:]
	} else if contentLength >= 0 {
		bodyEnd := bodyOffset + int(contentLength)
		if len(ctx.buf) < bodyEnd {
			return gnet.None
		}
		ctx.buf = ctx.buf[bodyEnd:]
	} else {
		return gnet.None
	}

	latency := time.Since(ctx.requestStart)
	cl.recordMetrics(ctx.parser.Status, latency, uint64(bodyOffset))

	shouldClose := ctx.parser.ConnectionClose
	ctx.parser.Reset()
	ctx.headerParsed = false
	ctx.bodyOffset = 0

	reqNum := atomic.AddUint64(&cl.metrics.completedRequests, 1)

	if atomic.LoadInt32(&cl.stopFlag) == 1 {
		return gnet.Close
	}
	if cl.reqLimit > 0 && reqNum >= cl.reqLimit {
		atomic.StoreInt32(&cl.stopFlag, 1)
		cl.closeLimitOnce.Do(func() {
			close(cl.limitReached)
		})
		return gnet.Close
	}
	if shouldClose {
		return gnet.Close
	}

	ctx.requestStart = time.Now()
	atomic.AddUint64(&cl.metrics.bytesWritten, uint64(len(cl.requestBytes)))
	c.AsyncWrite(cl.requestBytes, nil)

	return gnet.None
}

func main() {
	var urlStr string
	var concurrency int
	var totalRequests uint64
	var durationStr string

	flag.StringVar(&urlStr, "url", "http://localhost:80/pixel.gif", "target URL to benchmark")
	flag.IntVar(&concurrency, "c", 10, "number of concurrent connections")
	flag.Uint64Var(&totalRequests, "n", 0, "total requests to complete (0 for unlimited, duration-only)")
	flag.StringVar(&durationStr, "d", "10s", "duration of the benchmark")
	flag.Parse()

	u, err := url.Parse(urlStr)
	if err != nil {
		log.Fatalf("Invalid URL: %v", err)
	}

	host := u.Hostname()
	port := u.Port()
	if port == "" {
		port = "80"
	}
	path := u.RequestURI()

	duration, err := time.ParseDuration(durationStr)
	if err != nil {
		log.Fatalf("Invalid duration: %v", err)
	}

	addr := fmt.Sprintf("%s:%s", host, port)
	requestBytes := []byte(fmt.Sprintf("GET %s HTTP/1.1\r\nHost: %s\r\nConnection: keep-alive\r\n\r\n", path, host))

	cl := &httpClient{
		addr:         addr,
		reqLimit:     totalRequests,
		limitReached: make(chan struct{}),
		requestBytes: requestBytes,
	}

	cli, err := gnet.NewClient(cl, gnet.WithMulticore(true))
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	cl.cli = cli

	err = cli.Start()
	if err != nil {
		log.Fatalf("Failed to start event engine: %v", err)
	}

	startTime := time.Now()

	for i := 0; i < concurrency; i++ {
		_, err = cli.Dial("tcp", addr)
		if err != nil {
			log.Fatalf("Failed to dial connection %d: %v", i, err)
		}
	}

	select {
	case <-time.After(duration):
		atomic.StoreInt32(&cl.stopFlag, 1)
	case <-cl.limitReached:
		atomic.StoreInt32(&cl.stopFlag, 1)
	}

	cli.Stop()

	actualDuration := time.Since(startTime)
	completed := atomic.LoadUint64(&cl.metrics.completedRequests)
	success := atomic.LoadUint64(&cl.metrics.successRequests)
	failed := atomic.LoadUint64(&cl.metrics.failRequests)
	readBytes := atomic.LoadUint64(&cl.metrics.bytesRead)
	writeBytes := atomic.LoadUint64(&cl.metrics.bytesWritten)

	rps := float64(completed) / actualDuration.Seconds()
	mbRead := float64(readBytes) / (1024 * 1024)
	mbWrite := float64(writeBytes) / (1024 * 1024)
	throughputRead := mbRead / actualDuration.Seconds()
	throughputWrite := mbWrite / actualDuration.Seconds()

	minLat := time.Duration(atomic.LoadInt64(&cl.minLatency))
	maxLat := time.Duration(atomic.LoadInt64(&cl.maxLatency))
	meanLat := cl.meanLatency()
	p50 := cl.calculatePercentile(50.0)
	p90 := cl.calculatePercentile(90.0)
	p99 := cl.calculatePercentile(99.0)

	fmt.Println("================ GORK BENCHMARK SUMMARY ================")
	fmt.Printf("Target URL:           %s\n", urlStr)
	fmt.Printf("Concurrency Level:    %d\n", concurrency)
	fmt.Printf("Actual Duration:      %v\n\n", actualDuration.Round(time.Millisecond))
	fmt.Printf("Completed Requests:   %d\n", completed)
	fmt.Printf("Successful Responses: %d\n", success)
	fmt.Printf("Failed Responses:     %d\n", failed)
	fmt.Printf("Requests Per Second:  %.2f rps\n", rps)
	fmt.Printf("Read Throughput:      %.2f MB/s (%.2f MB total)\n", throughputRead, mbRead)
	fmt.Printf("Write Throughput:     %.2f MB/s (%.2f MB total)\n\n", throughputWrite, mbWrite)
	fmt.Println("Latency Profile:")
	fmt.Printf("  Min:                %v\n", minLat.Round(time.Microsecond))
	fmt.Printf("  Mean:               %v\n", meanLat.Round(time.Microsecond))
	fmt.Printf("  P50 (Median):       %v\n", p50.Round(time.Microsecond))
	fmt.Printf("  P90:                %v\n", p90.Round(time.Microsecond))
	fmt.Printf("  P99:                %v\n", p99.Round(time.Microsecond))
	fmt.Printf("  Max:                %v\n", maxLat.Round(time.Microsecond))
	fmt.Println("=========================================================")
}
