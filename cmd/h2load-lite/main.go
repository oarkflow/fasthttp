package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	url := flag.String("url", "https://127.0.0.1:8443/", "target URL; HTTPS will negotiate HTTP/2 automatically")
	method := flag.String("method", "GET", "HTTP method")
	body := flag.String("body", "", "request body")
	requests := flag.Int("n", 10000, "total requests")
	concurrency := flag.Int("c", 100, "concurrent workers")
	timeout := flag.Duration("timeout", 10*time.Second, "per-request timeout")
	insecure := flag.Bool("k", true, "skip TLS verification for local tests")
	flag.Parse()

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: *insecure},
		ForceAttemptHTTP2: true,
		MaxIdleConns: *concurrency * 2,
		MaxIdleConnsPerHost: *concurrency * 2,
		IdleConnTimeout: 90 * time.Second,
		DialContext: (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
	}
	client := &http.Client{Transport: transport}

	jobs := make(chan int)
	latencies := make([]time.Duration, 0, *requests)
	var latMu sync.Mutex
	var ok, failed atomic.Int64

	start := time.Now()
	var wg sync.WaitGroup
	for i := 0; i < *concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range jobs {
				ctx, cancel := context.WithTimeout(context.Background(), *timeout)
				var rdr io.Reader
				if *body != "" {
					rdr = bytes.NewBufferString(*body)
				}
				req, err := http.NewRequestWithContext(ctx, *method, *url, rdr)
				if err != nil {
					cancel()
					failed.Add(1)
					continue
				}
				if *body != "" {
					req.Header.Set("Content-Type", "text/plain")
				}
				t0 := time.Now()
				resp, err := client.Do(req)
				lat := time.Since(t0)
				cancel()
				if err != nil {
					failed.Add(1)
					continue
				}
				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
				if resp.StatusCode >= 200 && resp.StatusCode < 500 {
					ok.Add(1)
				} else {
					failed.Add(1)
				}
				latMu.Lock()
				latencies = append(latencies, lat)
				latMu.Unlock()
			}
		}()
	}
	for i := 0; i < *requests; i++ {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	total := time.Since(start)

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	pct := func(p float64) time.Duration {
		if len(latencies) == 0 {
			return 0
		}
		idx := int(float64(len(latencies)-1) * p)
		return latencies[idx]
	}
	fmt.Printf("requests=%d ok=%d failed=%d duration=%s rps=%.2f\n", *requests, ok.Load(), failed.Load(), total, float64(*requests)/total.Seconds())
	fmt.Printf("latency p50=%s p90=%s p95=%s p99=%s max=%s\n", pct(0.50), pct(0.90), pct(0.95), pct(0.99), pct(1.0))
	if len(latencies) > 0 {
		fmt.Printf("protocol=http/2 expected; verify server logs or add resp.Proto sampling if needed\n")
	}
}
