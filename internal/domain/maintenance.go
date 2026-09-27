package domain

import (
	"sort"
	"time"
)

const MaxMaintenanceWindows = 32

type MaintenanceWindow struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

func (w MaintenanceWindow) Contains(at time.Time) bool {
	return !at.Before(w.Start) && at.Before(w.End)
}

func (w MaintenanceWindow) Overlaps(other MaintenanceWindow) bool {
	return w.Start.Before(other.End) && other.Start.Before(w.End)
}

func NormalizeMaintenanceWindows(
	windows []MaintenanceWindow,
	now time.Time,
) ([]MaintenanceWindow, error) {
	if len(windows) > MaxMaintenanceWindows {
		return nil, FieldError(CodeInvalidInput, "maintenance_windows",
			"must contain at most 32 entries")
	}
	normalized := make([]MaintenanceWindow, 0, len(windows))
	for _, window := range windows {
		start := window.Start.UTC()
		end := window.End.UTC()
		if !end.After(start) {
			return nil, FieldError(CodeInvalidInput, "maintenance_windows",
				"window end must be after its start")
		}
		if !start.After(now.UTC()) {
			return nil, FieldError(CodeInvalidInput, "maintenance_windows",
				"window must start in the future")
		}
		normalized = append(normalized, MaintenanceWindow{Start: start, End: end})
	}
	sort.Slice(normalized, func(i, j int) bool {
		if normalized[i].Start.Equal(normalized[j].Start) {
			return normalized[i].End.Before(normalized[j].End)
		}
		return normalized[i].Start.Before(normalized[j].Start)
	})
	for index := 1; index < len(normalized); index++ {
		if normalized[index-1].Overlaps(normalized[index]) {
			return nil, NewError(CodeMaintenanceWindowConflict,
				"maintenance windows overlap on the same vessel")
		}
	}
	return normalized, nil
}
