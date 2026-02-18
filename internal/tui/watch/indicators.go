package watch

import "strings"

import "time"

// Ticker rotates through frames to show the system is alive.
// Stops rotating if no ticks arrive (indicates freeze).
type Ticker struct {
	frames   []string
	index    int
	lastTick time.Time
}

func NewTicker() Ticker {
	return Ticker{
		frames:   []string{"⟲", "⟳"},
		lastTick: time.Now(),
	}
}

func (t *Ticker) Tick() {
	t.index = (t.index + 1) % len(t.frames)
	t.lastTick = time.Now()
}

func (t Ticker) Current() string {
	return t.frames[t.index]
}

// Spinner shows event activity with a decaying dot pattern.
// Lights up on events, fades over time.
type Spinner struct {
	dots      int
	lastEvent time.Time
}

func NewSpinner() Spinner {
	return Spinner{}
}

func (s *Spinner) OnEvent() {
	s.dots = 5
	s.lastEvent = time.Now()
}

// Decay fades the spinner dots based on time since last event.
func (s *Spinner) Decay() {
	if s.dots == 0 {
		return
	}
	elapsed := time.Since(s.lastEvent)
	switch {
	case elapsed > 10*time.Second:
		s.dots = 0
	case elapsed > 8*time.Second:
		s.dots = 1
	case elapsed > 6*time.Second:
		s.dots = 2
	case elapsed > 4*time.Second:
		s.dots = 3
	case elapsed > 2*time.Second:
		s.dots = 4
	}
}

func (s Spinner) Render(theme Theme) string {
	var result strings.Builder
	for i := range 5 {
		if i < s.dots {
			result.WriteString(theme.TickerActive.Render("●"))
		} else {
			result.WriteString(theme.TickerInactive.Render("○"))
		}
	}
	return result.String()
}

func (s Spinner) LastEvent() time.Time {
	return s.lastEvent
}
