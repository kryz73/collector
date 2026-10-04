package quota

import (
	"log/slog"
	"os"
	"testing"
)

func TestGovernorSpend(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	g := NewGovernor(100, logger)

	if !g.CanSpend(50, PriorityNormal) {
		t.Errorf("Expected CanSpend(50) to be true")
	}

	if !g.Spend(50, PriorityNormal) {
		t.Errorf("Expected Spend(50) to be true")
	}

	if g.Used() != 50 {
		t.Errorf("Expected Used() = 50, got %d", g.Used())
	}

	if g.Remaining() != 50 {
		t.Errorf("Expected Remaining() = 50, got %d", g.Remaining())
	}

	// At 85% used, Low priority should be blocked
	g.Spend(35, PriorityNormal) // total 85%
	if g.CanSpend(5, PriorityLow) {
		t.Errorf("Expected PriorityLow to be blocked at 85%% quota usage")
	}

	// But Critical priority should still be allowed
	if !g.CanSpend(5, PriorityCritical) {
		t.Errorf("Expected PriorityCritical to be allowed at 85%% quota usage")
	}

	// Exceeding limit should return false
	if g.Spend(20, PriorityCritical) {
		t.Errorf("Expected Spend to fail when exceeding limit (85 + 20 > 100)")
	}
}

func TestGovernorCommentPagesScaling(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	g := NewGovernor(100, logger)

	if pages := g.MaxCommentPages(5); pages != 5 {
		t.Errorf("Expected 5 pages below 70%% usage, got %d", pages)
	}

	g.Spend(75, PriorityNormal)
	if pages := g.MaxCommentPages(5); pages != 2 {
		t.Errorf("Expected 2 pages between 70%% and 85%% usage, got %d", pages)
	}

	g.Spend(12, PriorityNormal) // total 87%
	if pages := g.MaxCommentPages(5); pages != 1 {
		t.Errorf("Expected 1 page above 85%% usage, got %d", pages)
	}
}
