package activity

import (
	"strings"
	"time"

	"github.com/mattjoyce/ductile/internal/tui/styles"
)

// Heartbeat shows scheduler ticks with a fading heart indicator.
// Ported from internal/tui/watch/indicators.go.
type Heartbeat struct {
	lastTick time.Time
}

func NewHeartbeat() Heartbeat {
	return Heartbeat{}
}

func (h *Heartbeat) OnTick() {
	h.lastTick = time.Now()
}

func (h Heartbeat) Render(now time.Time, interval time.Duration) string {
	if interval <= 0 {
		interval = time.Minute
	}
	if h.lastTick.IsZero() {
		return styles.HeartbeatDim.Render("♥")
	}

	elapsed := now.Sub(h.lastTick)
	step := interval / 3

	switch {
	case elapsed < step:
		return styles.HeartbeatActive.Render("♥")
	case elapsed < 2*step:
		return styles.HeaderBar.Render("♥")
	default:
		return styles.HeartbeatDim.Render("♥")
	}
}

// Dots shows event activity with a decaying dot pattern.
// Ported from internal/tui/watch/indicators.go.
type Dots struct {
	dots      int
	lastEvent time.Time
}

func NewDots() Dots {
	return Dots{}
}

func (d *Dots) OnEvent() {
	d.dots = 5
	d.lastEvent = time.Now()
}

// Decay fades the dots based on time since last event.
func (d *Dots) Decay() {
	if d.dots == 0 {
		return
	}
	elapsed := time.Since(d.lastEvent)
	switch {
	case elapsed > 10*time.Second:
		d.dots = 0
	case elapsed > 8*time.Second:
		d.dots = 1
	case elapsed > 6*time.Second:
		d.dots = 2
	case elapsed > 4*time.Second:
		d.dots = 3
	case elapsed > 2*time.Second:
		d.dots = 4
	}
}

func (d Dots) Render() string {
	var b strings.Builder
	for i := range 5 {
		if i < d.dots {
			b.WriteString(styles.ActivityWhite.Render("●"))
		} else {
			b.WriteString(styles.ActivityDim.Render("○"))
		}
	}
	return b.String()
}

func (d Dots) LastEvent() time.Time {
	return d.lastEvent
}
