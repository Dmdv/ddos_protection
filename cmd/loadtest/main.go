package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/ddos_protection/internal/pow"
	"github.com/ddos_protection/internal/protocol"
)

// Stats tracks load test statistics.
type Stats struct {
	totalRequests   atomic.Int64
	successRequests atomic.Int64
	failedRequests  atomic.Int64
	totalSolveTime  atomic.Int64 // nanoseconds
	totalRoundTrip  atomic.Int64 // nanoseconds
}

func main() {
	serverAddr := flag.String("server", "localhost:8080", "Server address")
	workers := flag.Int("workers", 10, "Number of concurrent workers")
	duration := flag.Duration("duration", 30*time.Second, "Test duration")
	rampUp := flag.Duration("ramp-up", 5*time.Second, "Ramp-up time to start all workers")
	connTimeout := flag.Duration("conn-timeout", 10*time.Second, "Connection timeout")
	flag.Parse()

	fmt.Println("=== Word of Wisdom Load Test ===")
	fmt.Printf("Server:   %s\n", *serverAddr)
	fmt.Printf("Workers:  %d\n", *workers)
	fmt.Printf("Duration: %s\n", *duration)
	fmt.Printf("Ramp-up:  %s\n", *rampUp)
	fmt.Println()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nShutting down...")
		cancel()
	}()

	stats := &Stats{}
	var wg sync.WaitGroup

	startTime := time.Now()
	workerDelay := *rampUp / time.Duration(*workers)

	// Start workers with ramp-up delay
	for i := 0; i < *workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			runWorker(ctx, id, *serverAddr, *connTimeout, *duration, startTime, stats)
		}(i)

		if i < *workers-1 {
			time.Sleep(workerDelay)
		}
	}

	// Print stats periodically
	go printLiveStats(ctx, stats, startTime)

	// Wait for duration or cancellation
	select {
	case <-ctx.Done():
	case <-time.After(*duration):
		cancel()
	}

	wg.Wait()

	// Final stats
	printFinalStats(stats, time.Since(startTime))
}

func runWorker(ctx context.Context, id int, serverAddr string, connTimeout, duration time.Duration, startTime time.Time, stats *Stats) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Check if we've exceeded duration
		if time.Since(startTime) > duration {
			return
		}

		err := doRequest(ctx, serverAddr, connTimeout, stats)
		if err != nil {
			stats.failedRequests.Add(1)
			// Brief backoff on error
			select {
			case <-ctx.Done():
				return
			case <-time.After(100 * time.Millisecond):
			}
		}
	}
}

func doRequest(ctx context.Context, serverAddr string, connTimeout time.Duration, stats *Stats) error {
	roundTripStart := time.Now()
	stats.totalRequests.Add(1)

	// Connect
	dialer := &net.Dialer{Timeout: connTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", serverAddr)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer conn.Close()

	// Set overall deadline
	_ = conn.SetDeadline(time.Now().Add(60 * time.Second))

	// Request challenge
	if err := protocol.WriteMessage(conn, protocol.NewRequestChallenge()); err != nil {
		return fmt.Errorf("send request: %w", err)
	}

	// Receive challenge
	msg, err := protocol.ReadMessage(conn)
	if err != nil {
		return fmt.Errorf("read challenge: %w", err)
	}

	if msg.Type == protocol.TypeError {
		if len(msg.Payload) > 0 {
			return fmt.Errorf("server error: %s", protocol.ErrorCode(msg.Payload[0]))
		}
		return fmt.Errorf("server error: unknown")
	}

	if msg.Type != protocol.TypeChallenge {
		return fmt.Errorf("unexpected message type: %d", msg.Type)
	}

	// Parse challenge
	challenge, err := pow.UnmarshalChallenge(msg.Payload)
	if err != nil {
		return fmt.Errorf("parse challenge: %w", err)
	}

	// Solve challenge
	solver := pow.NewSolver()
	solveStart := time.Now()

	solution, err := solver.Solve(ctx, challenge)
	if err != nil {
		return fmt.Errorf("solve: %w", err)
	}

	solveDuration := time.Since(solveStart)
	stats.totalSolveTime.Add(int64(solveDuration))

	// Send solution
	solutionBytes := solution.Marshal()
	if err := protocol.WriteMessage(conn, protocol.NewSolution(solutionBytes)); err != nil {
		return fmt.Errorf("send solution: %w", err)
	}

	// Receive quote
	msg, err = protocol.ReadMessage(conn)
	if err != nil {
		return fmt.Errorf("read quote: %w", err)
	}

	if msg.Type == protocol.TypeError {
		if len(msg.Payload) > 0 {
			return fmt.Errorf("server error: %s", protocol.ErrorCode(msg.Payload[0]))
		}
		return fmt.Errorf("server error: unknown")
	}

	if msg.Type != protocol.TypeQuote {
		return fmt.Errorf("unexpected message type: %d", msg.Type)
	}

	stats.successRequests.Add(1)
	stats.totalRoundTrip.Add(int64(time.Since(roundTripStart)))

	return nil
}

func printLiveStats(ctx context.Context, stats *Stats, startTime time.Time) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			elapsed := time.Since(startTime).Seconds()
			total := stats.totalRequests.Load()
			success := stats.successRequests.Load()
			failed := stats.failedRequests.Load()

			rps := float64(total) / elapsed
			successRate := float64(0)
			if total > 0 {
				successRate = float64(success) / float64(total) * 100
			}

			avgSolve := time.Duration(0)
			avgRoundTrip := time.Duration(0)
			if success > 0 {
				avgSolve = time.Duration(stats.totalSolveTime.Load() / success)
				avgRoundTrip = time.Duration(stats.totalRoundTrip.Load() / success)
			}

			fmt.Printf("[%5.1fs] Requests: %d | Success: %d (%.1f%%) | Failed: %d | RPS: %.1f | Avg Solve: %v | Avg RT: %v\n",
				elapsed, total, success, successRate, failed, rps, avgSolve.Truncate(time.Millisecond), avgRoundTrip.Truncate(time.Millisecond))
		}
	}
}

func printFinalStats(stats *Stats, duration time.Duration) {
	fmt.Println()
	fmt.Println("=== Final Results ===")

	total := stats.totalRequests.Load()
	success := stats.successRequests.Load()
	failed := stats.failedRequests.Load()

	fmt.Printf("Duration:        %v\n", duration.Truncate(time.Millisecond))
	fmt.Printf("Total Requests:  %d\n", total)
	fmt.Printf("Successful:      %d\n", success)
	fmt.Printf("Failed:          %d\n", failed)

	if total > 0 {
		successRate := float64(success) / float64(total) * 100
		rps := float64(total) / duration.Seconds()
		fmt.Printf("Success Rate:    %.2f%%\n", successRate)
		fmt.Printf("Requests/sec:    %.2f\n", rps)
	}

	if success > 0 {
		avgSolve := time.Duration(stats.totalSolveTime.Load() / success)
		avgRoundTrip := time.Duration(stats.totalRoundTrip.Load() / success)
		fmt.Printf("Avg Solve Time:  %v\n", avgSolve.Truncate(time.Millisecond))
		fmt.Printf("Avg Round Trip:  %v\n", avgRoundTrip.Truncate(time.Millisecond))
	}

	fmt.Println()
}
