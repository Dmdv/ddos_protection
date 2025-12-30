package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/ddos_protection/internal/pow"
	"github.com/ddos_protection/internal/protocol"
)

func main() {
	// Parse command line flags
	serverAddr := flag.String("server", "localhost:8080", "Server address")
	timeout := flag.Duration("timeout", 60*time.Second, "Connection timeout")
	verbose := flag.Bool("verbose", false, "Verbose output")
	flag.Parse()

	if err := run(*serverAddr, *timeout, *verbose); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(serverAddr string, timeout time.Duration, verbose bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if verbose {
		fmt.Printf("Connecting to %s...\n", serverAddr)
	}

	// Connect to server
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", serverAddr)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer conn.Close()

	if verbose {
		fmt.Println("Connected. Requesting challenge...")
	}

	// Set connection deadline
	_ = conn.SetDeadline(time.Now().Add(timeout))

	// Step 1: Request challenge
	if err := protocol.WriteMessage(conn, protocol.NewRequestChallenge()); err != nil {
		return fmt.Errorf("send request: %w", err)
	}

	// Step 2: Receive challenge
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

	if verbose {
		fmt.Printf("Received challenge (difficulty: %d)\n", challenge.Difficulty)
		fmt.Println("Solving challenge...")
	}

	// Step 3: Solve the challenge
	solver := pow.NewSolver()
	startTime := time.Now()

	solution, err := solver.Solve(ctx, challenge)
	if err != nil {
		return fmt.Errorf("solve challenge: %w", err)
	}

	solveDuration := time.Since(startTime)

	if verbose {
		fmt.Printf("Challenge solved in %v (counter: %d)\n", solveDuration, solution.Counter)
		fmt.Println("Sending solution...")
	}

	// Step 4: Send solution
	solutionBytes := solution.Marshal()
	if err := protocol.WriteMessage(conn, protocol.NewSolution(solutionBytes)); err != nil {
		return fmt.Errorf("send solution: %w", err)
	}

	// Step 5: Receive quote
	msg, err = protocol.ReadMessage(conn)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
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

	// Print the quote
	quote := string(msg.Payload)
	if verbose {
		fmt.Println("\nWord of Wisdom:")
		fmt.Println("---")
	}
	fmt.Println(quote)
	if verbose {
		fmt.Println("---")
	}

	return nil
}
