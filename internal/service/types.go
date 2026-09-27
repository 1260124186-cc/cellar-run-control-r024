package service

import (
	"time"

	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/domain"
)

type CreateFormulaInput struct {
	Name                  string
	Style                 string
	TargetOriginalGravity float64
	TargetFinalGravity    float64
	MaxFermentationTempC  float64
	MinimumDays           int
	MaximumDays           int
	Tags                  []string
}

type SetFormulaStepsInput struct {
	ExpectedVersion *int64
	Steps           []domain.FormulaStep
}

type SetFormulaTagsInput struct {
	ExpectedVersion *int64
	Tags            []string
}

// FormulaListQuery describes one formula list request. Filters combine with
// AND. Tags must all be present on a formula to match. Page and PageSize are
// both zero for an unpaged response; otherwise both must be positive.
type FormulaListQuery struct {
	Name     string
	Style    string
	State    string
	Tags     []string
	Page     int
	PageSize int
}

type FormulaListResult struct {
	Formulas []domain.Formula
	Total    int
	Page     int
	PageSize int
	Revision int64
}

type RenameTagInput struct {
	// ExpectedRevision is the snapshot revision the caller based the rename
	// on; it is required so concurrent renames surface a revision conflict.
	ExpectedRevision *int64
	From             string
	To               string
}

type RenameTagResult struct {
	From             string
	To               string
	FormulasAffected int
	Revision         int64
}

type TagCount struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}

// TagDirectory is the tag list plus the snapshot revision it was read from.
// The revision is the expected version a client supplies to POST
// /v1/tags/rename so a concurrent rename fails with a revision conflict.
type TagDirectory struct {
	Tags     []TagCount
	Revision int64
}

type VersionInput struct {
	ExpectedVersion *int64
}

type CreateVesselInput struct {
	Code      string
	CapacityL float64
}

type CreateRunInput struct {
	Code           string
	FormulaID      string
	PlannedVolumeL float64
	Notes          string
}

type ReserveRunInput struct {
	ExpectedVersion *int64
	VesselID        string
}

type ObservationInput struct {
	Sequence     int
	Gravity      float64
	TemperatureC float64
	ObservedAt   time.Time
	Note         string
}

type FinishCleaningInput struct {
	ExpectedVersion *int64
}
