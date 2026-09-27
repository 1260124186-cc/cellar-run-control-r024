package service

import (
	"testing"
	"time"

	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/clock"
	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/domain"
	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/storage"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	store, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	return New(store, clock.NewFixed(time.Date(2026, 9, 27, 8, 0, 0, 0, time.UTC)))
}

func validFormulaInput(name, style string, tags ...string) CreateFormulaInput {
	return CreateFormulaInput{
		Name:                  name,
		Style:                 style,
		TargetOriginalGravity: 1.050,
		TargetFinalGravity:    1.010,
		MaxFermentationTempC:  20,
		MinimumDays:           5,
		MaximumDays:           10,
		Tags:                  tags,
	}
}

func TestListFormulasFiltersAndPagesConsistently(t *testing.T) {
	svc := newTestService(t)
	specs := []struct {
		name  string
		style string
		tags  []string
	}{
		{"Alpha Ale", "Pale", []string{"summer"}},
		{"Bravo Stout", "Dark", []string{"winter", "dark"}},
		{"Charlie Saison", "Pale", []string{"summer", "belgian"}},
	}
	for _, spec := range specs {
		if _, err := svc.CreateFormula(validFormulaInput(spec.name, spec.style, spec.tags...)); err != nil {
			t.Fatalf("create %s: %v", spec.name, err)
		}
	}

	paleSummer, err := svc.ListFormulas(FormulaListQuery{Style: "pale", Tags: []string{"summer"}})
	if err != nil {
		t.Fatalf("filtered list: %v", err)
	}
	if paleSummer.Total != 2 {
		t.Fatalf("expected 2 pale summer formulas, got %d", paleSummer.Total)
	}

	// Paged totals must equal the filtered count and pages concatenate in
	// stable name order.
	page1, err := svc.ListFormulas(FormulaListQuery{
		Style: "pale", Tags: []string{"summer"}, Page: 1, PageSize: 1,
	})
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	page2, err := svc.ListFormulas(FormulaListQuery{
		Style: "pale", Tags: []string{"summer"}, Page: 2, PageSize: 1,
	})
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	if page1.Total != 2 || page2.Total != 2 {
		t.Fatalf("page totals drifted: %d %d", page1.Total, page2.Total)
	}
	if page1.Formulas[0].Name != "Alpha Ale" || page2.Formulas[0].Name != "Charlie Saison" {
		t.Fatalf("page order was not stable: %s %s",
			page1.Formulas[0].Name, page2.Formulas[0].Name)
	}

	// A page beyond the last row is empty but keeps the same total.
	beyond, err := svc.ListFormulas(FormulaListQuery{Page: 9, PageSize: 1})
	if err != nil {
		t.Fatalf("trailing page: %v", err)
	}
	if beyond.Total != 3 || len(beyond.Formulas) != 0 {
		t.Fatalf("trailing page mismatch: total=%d rows=%d", beyond.Total, len(beyond.Formulas))
	}
}

func TestListFormulasRejectsInvalidPagination(t *testing.T) {
	svc := newTestService(t)
	for _, query := range []FormulaListQuery{
		{Page: 1, PageSize: 0},
		{Page: 0, PageSize: 5},
		{Page: -1, PageSize: 5},
		{Page: 1, PageSize: 201},
	} {
		if _, err := svc.ListFormulas(query); err == nil ||
			!domain.IsCode(err, domain.CodeInvalidPagination) {
			t.Fatalf("expected invalid_pagination for %+v, got %v", query, err)
		}
	}
}

func TestRenameFormulaTagGuardsRevisionAndContent(t *testing.T) {
	svc := newTestService(t)
	formula, err := svc.CreateFormula(validFormulaInput("Tagged Ale", "pilot", "summer"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	steps := []domain.FormulaStep{{
		Sequence: 1, Label: "Hold", TemperatureC: 20, HoldMinutes: 30,
	}}
	withSteps, err := svc.SetFormulaSteps(formula.ID,
		SetFormulaStepsInput{ExpectedVersion: &formula.Version, Steps: steps})
	if err != nil {
		t.Fatalf("set steps: %v", err)
	}
	approved, err := svc.ApproveFormula(formula.ID,
		VersionInput{ExpectedVersion: &withSteps.Version})
	if err != nil {
		t.Fatalf("approve: %v", err)
	}

	directory, err := svc.ListFormulaTags()
	if err != nil {
		t.Fatalf("tag directory: %v", err)
	}
	if directory.Revision != svc.Revision() {
		t.Fatalf("directory revision %d does not match store %d",
			directory.Revision, svc.Revision())
	}

	// A stale revision conflicts even though the rename would otherwise apply.
	stale := int64(directory.Revision - 1)
	if _, err := svc.RenameFormulaTag(RenameTagInput{
		ExpectedRevision: &stale, From: "summer", To: "harvest",
	}); err == nil || !domain.IsCode(err, domain.CodeRevisionConflict) {
		t.Fatalf("expected revision conflict, got %v", err)
	}

	result, err := svc.RenameFormulaTag(RenameTagInput{
		ExpectedRevision: &directory.Revision, From: " Summer ", To: "Harvest",
	})
	if err != nil {
		t.Fatalf("rename: %v", err)
	}
	if result.FormulasAffected != 1 || result.Revision != directory.Revision+1 {
		t.Fatalf("unexpected rename result: %+v", result)
	}

	// The approved formula keeps its process content but answers to the new
	// tag; version and updated_at moved, steps and targets did not.
	read, err := svc.GetFormula(formula.ID)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if read.State != domain.FormulaApproved {
		t.Fatalf("tag rename changed formula state")
	}
	if len(read.Steps) != 1 || read.Steps[0] != steps[0] {
		t.Fatalf("tag rename changed approved steps: %+v", read.Steps)
	}
	if read.TargetOriginalGravity != approved.TargetOriginalGravity {
		t.Fatalf("tag rename changed gravity target")
	}
	if len(read.Tags) != 1 || !domain.HasTag(read.Tags, "harvest") || domain.HasTag(read.Tags, "summer") {
		t.Fatalf("tags after rename: %v", read.Tags)
	}

	// Unknown source tag and colliding target are explicit failures.
	current := result.Revision
	if _, err := svc.RenameFormulaTag(RenameTagInput{
		ExpectedRevision: &current, From: "ghost", To: "phantom",
	}); err == nil || !domain.IsCode(err, domain.CodeTagNotFound) {
		t.Fatalf("expected tag_not_found, got %v", err)
	}

	// Add a second formula carrying the destination tag, then renaming onto
	// it must be a duplicate_tag conflict without partial writes.
	other, err := svc.CreateFormula(validFormulaInput("Other Ale", "pilot", "harvest"))
	if err != nil {
		t.Fatalf("create second formula: %v", err)
	}
	otherRevision := other.Version
	if _, err := svc.SetFormulaTags(other.ID, SetFormulaTagsInput{
		ExpectedVersion: &otherRevision, Tags: []string{"harvest", "dark"},
	}); err != nil {
		t.Fatalf("tag second formula: %v", err)
	}
	latest := svc.Revision()
	if _, err := svc.RenameFormulaTag(RenameTagInput{
		ExpectedRevision: &latest, From: "dark", To: "harvest",
	}); err == nil || !domain.IsCode(err, domain.CodeDuplicateTag) {
		t.Fatalf("expected duplicate_tag on collision, got %v", err)
	}
	after, err := svc.GetFormula(other.ID)
	if err != nil {
		t.Fatalf("read after failed rename: %v", err)
	}
	if len(after.Tags) != 2 || !domain.HasTag(after.Tags, "dark") {
		t.Fatalf("failed rename left partial writes: %v", after.Tags)
	}
}

func TestConcurrentRenamesOnlyOneWins(t *testing.T) {
	svc := newTestService(t)
	if _, err := svc.CreateFormula(validFormulaInput("Concurrent Ale", "Pale", "shared")); err != nil {
		t.Fatalf("create: %v", err)
	}
	revision := svc.Revision()
	const contenders = 8
	wins := make(chan error, contenders)
	start := make(chan struct{})
	for index := 0; index < contenders; index++ {
		go func(suffix int) {
			<-start
			target := "renamed" + string(rune('a'+suffix))
			_, err := svc.RenameFormulaTag(RenameTagInput{
				ExpectedRevision: &revision, From: "shared", To: target,
			})
			wins <- err
		}(index)
	}
	close(start)
	successes, conflicts := 0, 0
	for index := 0; index < contenders; index++ {
		err := <-wins
		switch {
		case err == nil:
			successes++
		case domain.IsCode(err, domain.CodeRevisionConflict):
			conflicts++
		default:
			t.Fatalf("unexpected concurrent rename error: %v", err)
		}
	}
	if successes != 1 || conflicts != contenders-1 {
		t.Fatalf("expected exactly one winning rename, got %d wins and %d conflicts",
			successes, conflicts)
	}
}
