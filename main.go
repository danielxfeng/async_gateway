package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/danielxfeng/async_gateway/core"
	"github.com/getsentry/sentry-go"
)

var registeredServices []core.Service

func initSentry() error {
	env := os.Getenv("ENVIRONMENT")

	if env == "production" {
		dsn := os.Getenv("SENTRY_DSN")
		if dsn == "" {
			return fmt.Errorf("SENTRY_DSN environment variable is not set")
		}

		if err := sentry.Init(sentry.ClientOptions{
			Dsn: dsn,
		}); err != nil {
			return fmt.Errorf("sentry.Init: %w", err)
		}
	}

	return nil
}

func runOnce(serv core.Service, done chan<- error) {
	// Handle panic
	defer func() {
		if r := recover(); r != nil {
			done <- fmt.Errorf("panic: %v", r)
		}
	}()

	done <- serv.Run()
}

func serviceRunner(serv core.Service, ctx context.Context, wg *sync.WaitGroup, retry int, wait time.Duration) {
	wg.Add(1)

	go func() {
		defer wg.Done()

		for {
			done := make(chan error, 1)
			go runOnce(serv, done)

			select {
			case <-ctx.Done():
				fmt.Printf("Service %s: shutdown signal received\n", serv.Name())
				serv.Clean()
				return

			case err := <-done:
				if err == nil {
					fmt.Printf("Service %s exited normally\n", serv.Name())
					return
				}

				log.Printf("service %s failed with error: %v.", serv.Name(), err)
				sentry.CaptureException(err)

				retry--
				if retry <= 0 {
					log.Printf("Service %s no more retries, stopping", serv.Name())
					return
				}

				time.Sleep(wait)
			}
		}
	}()
}

func run(registeredServices []core.Service, retry int, wait time.Duration) {
	// init sentry
	err := initSentry()
	if err != nil {
		log.Fatalf("Failed to initialize Sentry: %v", err)
	}
	defer sentry.Flush(2 * time.Second)

	// signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("Shutting down...")
		cancel()
	}()

	// wait group for services
	var wg sync.WaitGroup

	for _, serv := range registeredServices {
		serviceRunner(serv, ctx, &wg, retry, wait)
	}

	// Quit only after all services are panicked or received a shutdown signal
	wg.Wait()
	fmt.Println("All services have been shut down gracefully.")
}

func main() {
	retry := 3
	retryStr := os.Getenv("RETRY")
	retryInt, err := strconv.Atoi(retryStr)
	if err == nil && retryInt > 0 {
		retry = retryInt
	}

	run(registeredServices, retry, 10*time.Second)
}
