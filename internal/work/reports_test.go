package work

import (
	"testing"
	"time"

	"cairn/internal/model"
	"cairn/internal/store"
)

func day(s string) time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return t
}

func TestCategoryAsOf(t *testing.T) {
	tl := []store.StatusChange{
		{Category: model.CategoryTodo, ChangedAt: day("2026-06-01")},
		{Category: model.CategoryInProgress, ChangedAt: day("2026-06-03")},
		{Category: model.CategoryDone, ChangedAt: day("2026-06-05")},
	}
	cases := []struct {
		when string
		want string
	}{
		{"2026-05-31", ""},                        // before the issue existed
		{"2026-06-01", model.CategoryTodo},        // created
		{"2026-06-04", model.CategoryInProgress},  // mid-flight
		{"2026-06-10", model.CategoryDone},        // after final change
	}
	for _, c := range cases {
		if got := categoryAsOf(tl, endOfDay(day(c.when))); got != c.want {
			t.Errorf("categoryAsOf(%s) = %q, want %q", c.when, got, c.want)
		}
	}
}

func TestDaysBetweenInclusive(t *testing.T) {
	days := daysBetween(day("2026-06-01"), day("2026-06-04"))
	if len(days) != 4 {
		t.Fatalf("expected 4 days, got %d", len(days))
	}
	if days[0].Format(dayFmt) != "2026-06-01" || days[3].Format(dayFmt) != "2026-06-04" {
		t.Fatalf("unexpected bounds: %v..%v", days[0], days[3])
	}
}

func TestSprintWindowFallsBackToHistory(t *testing.T) {
	// No explicit dates → window starts at the first status change.
	sprint := &model.Sprint{}
	changes := []store.StatusChange{{Category: model.CategoryTodo, ChangedAt: day("2026-06-02")}}
	start, end := sprintWindow(sprint, changes)
	if start.Format(dayFmt) != "2026-06-02" {
		t.Errorf("start = %s, want 2026-06-02", start.Format(dayFmt))
	}
	if end.Before(start) {
		t.Errorf("end %v before start %v", end, start)
	}
}
