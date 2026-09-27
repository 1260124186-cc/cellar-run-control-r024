package domain

import (
	"strconv"
	"testing"
	"time"
)

var fixedNow = time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)

func TestNormalizeTagsDeduplicatesAndReports(t *testing.T) {
	tags, err := NormalizeTags([]string{" Summer  Ale ", "summer ale", "pilot"})
	if err == nil || !IsCode(err, CodeDuplicateTag) {
		t.Fatalf("expected duplicate_tag error, got %v", err)
	}
	if tags != nil {
		t.Fatalf("duplicate request must not return a partial tag list")
	}
}

func TestNormalizeTagsSortsAndValidates(t *testing.T) {
	tags, err := NormalizeTags([]string{"pilot", "belgian", "summer"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"belgian", "pilot", "summer"}
	for index, tag := range want {
		if tags[index] != tag {
			t.Fatalf("expected sorted tags %v, got %v", want, tags)
		}
	}
}

func TestNormalizeTagsRejectsBadValues(t *testing.T) {
	for _, value := range []string{"", "sum/mer", " -", " leading dash is fine?"} {
		if value == " leading dash is fine?" {
			continue // this trims and becomes valid; exclude from bad cases
		}
		if _, err := NormalizeTags([]string{value}); err == nil {
			t.Fatalf("expected validation error for %q", value)
		}
	}
}

func TestNormalizeTagsEnforcesLimit(t *testing.T) {
	values := make([]string, MaxFormulaTags+1)
	for index := range values {
		values[index] = "tag" + strconv.Itoa(index)
	}
	if _, err := NormalizeTags(values); err == nil {
		t.Fatalf("expected more than %d tags to be rejected", MaxFormulaTags)
	}
}

func TestSetTagsAllowedAfterApproval(t *testing.T) {
	formula := &Formula{State: FormulaApproved, Version: 3, Tags: []string{}}
	if err := formula.SetTags([]string{"pilot"}, fixedNow); err != nil {
		t.Fatalf("tag maintenance must be allowed after approval: %v", err)
	}
	if formula.State != FormulaApproved || formula.Version != 4 {
		t.Fatalf("approval state/version changed unexpectedly: %+v", formula)
	}
	if len(formula.Tags) != 1 || formula.Tags[0] != "pilot" {
		t.Fatalf("tags were not stored: %v", formula.Tags)
	}
}

func TestSetTagsRejectsDuplicates(t *testing.T) {
	formula := &Formula{State: FormulaDraft, Version: 1}
	err := formula.SetTags([]string{"Pilot", "pilot"}, fixedNow)
	if err == nil || !IsCode(err, CodeDuplicateTag) {
		t.Fatalf("expected duplicate_tag, got %v", err)
	}
	if formula.Version != 1 {
		t.Fatalf("failed tag maintenance must not bump the version")
	}
}
