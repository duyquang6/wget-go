package shutdown

import (
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

var (
	// GetSigtermHandler returns sigterm handler for graceful shutdown
	GetSigtermHandler = getSigtermHandlerFunc()
)

type sigtermHandler struct {
	functions  []func() error
	sigChannel chan os.Signal
	timeout    time.Duration
	mu         sync.Mutex
	done       chan struct{}
}

// SetTimeout set timeout for running deferred functions. It should be set before all of RegisterDeferFunc
func (s *sigtermHandler) SetTimeout(duration time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.timeout = duration
}

// RegisterFunc registers functions for calling when a sigterm signal is received
func (s *sigtermHandler) RegisterFunc(f func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.functions = append(s.functions, func() error {
		f()
		return nil
	})
}

// RegisterErrorFunc registers functions for calling when sigterm signal is received
func (s *sigtermHandler) RegisterErrorFunc(f func() error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.functions = append(s.functions, f)
}

// Wait waits until all registered functions are done
func (s *sigtermHandler) Wait() {
	<-s.done
	log.Println("Done!")
}

func getSigtermHandlerFunc() func() *sigtermHandler {
	var (
		sigtermHdl     *sigtermHandler
		sigHdlInitOnce sync.Once
	)
	return func() *sigtermHandler {
		sigHdlInitOnce.Do(func() {
			sigtermHdl = &sigtermHandler{
				functions:  make([]func() error, 0),
				sigChannel: make(chan os.Signal, 1),
				timeout:    -1,
				done:       make(chan struct{}),
			}
			signal.Notify(sigtermHdl.sigChannel, os.Interrupt, syscall.SIGTERM)
			signalsReceived := 0
			go func() {
				defer close(sigtermHdl.done)
				s := <-sigtermHdl.sigChannel
				sigtermHdl.mu.Lock()
				defer sigtermHdl.mu.Unlock()
				signalsReceived++
				log.Println("Received signal: ", s)
				if signalsReceived == 1 {
					if sigtermHdl.timeout > 0 {
						go func() {
							<-time.After(sigtermHdl.timeout)
							log.Println("Timeout! Force application to exit.")
							os.Exit(1)
						}()
					}
					log.Println("Waiting for gracefully finishing current works before shutdown...")
					for i := len(sigtermHdl.functions) - 1; i >= 0; i-- {
						if err := sigtermHdl.functions[i](); err != nil {
							log.Println("Shutdown error", err)
						}
					}
				} else {
					log.Println("Force application to exit...")
					os.Exit(1)
				}
			}()
		})
		return sigtermHdl
	}
}