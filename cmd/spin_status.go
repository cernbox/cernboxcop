package cmd

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/tj/go-spin"
)

const (
	DeterminatedSpin   = SpinType(0)
	IndeterminatedSpin = SpinType(1)
	DescriptionSpin    = SpinType(2)
)

type SpinType int

type SpinStatus struct {
	spin        *spin.Spinner
	current     int
	total       int
	mu          *sync.Mutex
	info        string
	description string
	ticker      *time.Ticker
	t           SpinType
}

func NewDeterminatedSpinStatus(info string, total int) *SpinStatus {
	return &SpinStatus{spin: spin.New(), current: 0, total: total, mu: &sync.Mutex{}, info: info, t: DeterminatedSpin}
}

func NewIndeterminatedSpinStatus(info string) *SpinStatus {
	return &SpinStatus{spin: spin.New(), mu: &sync.Mutex{}, info: info, t: IndeterminatedSpin}
}

func NewDescriptionSpinStatus(info string) *SpinStatus {
	return &SpinStatus{spin: spin.New(), mu: &sync.Mutex{}, info: info, t: DescriptionSpin}
}

func (s *SpinStatus) getDescription() string {
	switch s.t {
	case DeterminatedSpin:
		return fmt.Sprintf("[%d/%d]", s.current, s.total)
	case IndeterminatedSpin:
		return ""
	case DescriptionSpin:
		return s.description
	default:
		return ""
	}
}

func (s *SpinStatus) printStatus(status, end string) {
	fmt.Fprintf(os.Stderr, "\033[2K\r %s %s %s%s", status, s.info, s.getDescription(), end)
}

func (s *SpinStatus) refresh() {
	s.printStatus(s.spin.Current(), "")
}

func (s *SpinStatus) Start() {
	s.ticker = time.NewTicker(100 * time.Millisecond)
	go func() {
		for range s.ticker.C {
			s.spin.Next()
			s.mu.Lock()
			s.refresh()
			s.mu.Unlock()
		}
	}()
	s.refresh()
}

func (s *SpinStatus) Done() {
	s.ticker.Stop()
	s.printStatus("\033[32m✓\033[0m", "\n")
}

func (s *SpinStatus) Update(delta int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.current += delta
	s.refresh()
}

func (s *SpinStatus) UpdateDescription(newDesc string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.description = newDesc
	s.refresh()
}
