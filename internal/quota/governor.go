package quota

import (
	"log/slog"
	"sync"
	"time"
)

type Priority int

const (
	PriorityCritical Priority = 3 // Snapshots, Trending ground truth
	PriorityNormal   Priority = 2 // New video metadata discovery
	PriorityLow      Priority = 1 // Deep comment harvesting
)

type Governor struct {
	mu           sync.Mutex
	dailyLimit   int
	usedToday    int
	lastResetDay int
	loc          *time.Location
	logger       *slog.Logger
}

func NewGovernor(dailyLimit int, logger *slog.Logger) *Governor {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		loc = time.FixedZone("PST", -8*60*60)
	}

	g := &Governor{
		dailyLimit:   dailyLimit,
		usedToday:    0,
		lastResetDay: time.Now().In(loc).Day(),
		loc:          loc,
		logger:       logger,
	}
	return g
}

// checkReset checks if midnight Pacific Time has passed, resetting the daily counter.
func (g *Governor) checkResetLocked() {
	nowPT := time.Now().In(g.loc)
	currentDay := nowPT.Day()
	if currentDay != g.lastResetDay {
		g.logger.Info("Midnight Pacific Time passed, resetting daily quota counter",
			"previous_used", g.usedToday,
			"limit", g.dailyLimit,
			"reset_time_pt", nowPT.Format(time.RFC3339),
		)
		g.usedToday = 0
		g.lastResetDay = currentDay
	}
}

// CanSpend checks if the requested units can be spent according to priority and current usage.
func (g *Governor) CanSpend(units int, p Priority) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.checkResetLocked()

	if g.usedToday+units > g.dailyLimit {
		return false
	}

	usedPercent := float64(g.usedToday) / float64(g.dailyLimit)

	// If over 90% quota used, only Critical jobs (snapshots, trending) are allowed
	if usedPercent >= 0.90 && p < PriorityCritical {
		g.logger.Warn("Quota governor throttled request: >90% daily quota used, blocking non-critical job",
			"used_percent", usedPercent*100,
			"requested_units", units,
		)
		return false
	}

	// If over 80% quota used, Low priority (deep comments) is blocked
	if usedPercent >= 0.80 && p < PriorityNormal {
		g.logger.Warn("Quota governor throttled request: >80% daily quota used, blocking low-priority job",
			"used_percent", usedPercent*100,
			"requested_units", units,
		)
		return false
	}

	return true
}

// Spend records consumption of units if allowed. Returns true if spent, false if throttled.
func (g *Governor) Spend(units int, p Priority) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.checkResetLocked()

	if g.usedToday+units > g.dailyLimit {
		g.logger.Error("Daily quota exhausted, operation blocked",
			"used", g.usedToday,
			"requested", units,
			"limit", g.dailyLimit,
		)
		return false
	}

	usedPercent := float64(g.usedToday) / float64(g.dailyLimit)
	if usedPercent >= 0.90 && p < PriorityCritical {
		return false
	}
	if usedPercent >= 0.80 && p < PriorityNormal {
		return false
	}

	g.usedToday += units
	return true
}

// Used returns total units consumed today.
func (g *Governor) Used() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.checkResetLocked()
	return g.usedToday
}

// Remaining returns available quota units remaining today.
func (g *Governor) Remaining() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.checkResetLocked()
	rem := g.dailyLimit - g.usedToday
	if rem < 0 {
		return 0
	}
	return rem
}

// MaxCommentPages returns allowed comment pagination depth based on remaining quota.
func (g *Governor) MaxCommentPages(configuredMax int) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.checkResetLocked()

	usedPercent := float64(g.usedToday) / float64(g.dailyLimit)
	if usedPercent >= 0.85 {
		return 1 // Scale down to 1 page (100 comments) when running low
	}
	if usedPercent >= 0.70 {
		if configuredMax > 2 {
			return 2
		}
	}
	return configuredMax
}
