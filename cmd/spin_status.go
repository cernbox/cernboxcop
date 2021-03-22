package cmd

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/tj/go-spin"
)

type SpinStatus struct {
	spin    *spin.Spinner
	current int
	total   int
	mu      *sync.Mutex
	info    string
	ticker  *time.Ticker
}

func NewSpinStatus(info string, total int) *SpinStatus {
	return &SpinStatus{spin: spin.New(), current: 0, total: total, mu: &sync.Mutex{}, info: info}
}

func (s *SpinStatus) print() {
	fmt.Fprintf(os.Stderr, "\r %s %s [%d/%d]", s.spin.Current(), s.info, s.current, s.total)
}

func (s *SpinStatus) Start() {
	go func() {
		s.ticker = time.NewTicker(100 * time.Millisecond)
		for range s.ticker.C {
			s.spin.Next()
			s.mu.Lock()
			s.print()
			s.mu.Unlock()
		}
	}()
	s.print()
}

func (s *SpinStatus) Done() {
	s.ticker.Stop()
	fmt.Fprintf(os.Stderr, "\r \033[32m✓\033[0m %s [%d/%d]", s.info, s.total, s.total)
}

func (s *SpinStatus) Update(delta int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.current += delta
	s.print()
}
