package service

import (
	"math"
	"sort"
	"strings"
	"time"

	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/domain"
)

func (s *Service) CreateFormula(input CreateFormulaInput) (domain.Formula, error) {
	now := s.clock.Now()
	tags, err := domain.NormalizeTags(input.Tags)
	if err != nil {
		return domain.Formula{}, err
	}
	formula := domain.Formula{
		Name:                  input.Name,
		Style:                 input.Style,
		TargetOriginalGravity: input.TargetOriginalGravity,
		TargetFinalGravity:    input.TargetFinalGravity,
		MaxFermentationTempC:  input.MaxFermentationTempC,
		MinimumDays:           input.MinimumDays,
		MaximumDays:           input.MaximumDays,
		Steps:                 []domain.FormulaStep{},
		Tags:                  tags,
		State:                 domain.FormulaDraft,
		Version:               1,
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	formula.Normalize()
	if err := formula.Validate(); err != nil {
		return domain.Formula{}, err
	}
	id, err := generateEntityID("formula")
	if err != nil {
		return domain.Formula{}, domain.NewError(domain.CodeStorage, err.Error())
	}
	formula.ID = id
	nameKey := domain.NameKey(formula.Name)

	err = s.store.Execute(func(snapshot *domain.Snapshot) error {
		if _, exists := snapshot.NameIndex[nameKey]; exists {
			return domain.NewError(domain.CodeFormulaNameConflict,
				"a formula with the same normalized name already exists")
		}
		snapshot.Formulas[formula.ID] = formula
		snapshot.NameIndex[nameKey] = formula.ID
		return nil
	})
	if err != nil {
		return domain.Formula{}, err
	}
	return formula.Clone(), nil
}

func (s *Service) SetFormulaSteps(
	id string,
	input SetFormulaStepsInput,
) (domain.Formula, error) {
	var updated domain.Formula
	err := s.store.Execute(func(snapshot *domain.Snapshot) error {
		formula, ok := snapshot.Formulas[id]
		if !ok {
			return domain.NewError(domain.CodeFormulaNotFound, "formula was not found")
		}
		if err := s.checkExpected(formula.Version, input.ExpectedVersion); err != nil {
			return err
		}
		if err := formula.SetSteps(input.Steps); err != nil {
			return err
		}
		formula.UpdatedAt = s.clock.Now()
		snapshot.Formulas[id] = formula
		updated = formula.Clone()
		return nil
	})
	if err != nil {
		return domain.Formula{}, err
	}
	return updated, nil
}

func (s *Service) ApproveFormula(id string, input VersionInput) (domain.Formula, error) {
	var updated domain.Formula
	err := s.store.Execute(func(snapshot *domain.Snapshot) error {
		formula, ok := snapshot.Formulas[id]
		if !ok {
			return domain.NewError(domain.CodeFormulaNotFound, "formula was not found")
		}
		if err := s.checkExpected(formula.Version, input.ExpectedVersion); err != nil {
			return err
		}
		if err := formula.Approve(s.clock.Now()); err != nil {
			return err
		}
		snapshot.Formulas[id] = formula
		updated = formula.Clone()
		return nil
	})
	if err != nil {
		return domain.Formula{}, err
	}
	return updated, nil
}

func (s *Service) RetireFormula(id string, input VersionInput) (domain.Formula, error) {
	var updated domain.Formula
	err := s.store.Execute(func(snapshot *domain.Snapshot) error {
		formula, ok := snapshot.Formulas[id]
		if !ok {
			return domain.NewError(domain.CodeFormulaNotFound, "formula was not found")
		}
		if err := s.checkExpected(formula.Version, input.ExpectedVersion); err != nil {
			return err
		}
		for _, run := range snapshot.Runs {
			if run.FormulaID == id && run.State != domain.RunCompleted &&
				run.State != domain.RunAborted {
				return domain.NewError(domain.CodeFormulaInUse,
					"formula is attached to a run that is still open")
			}
		}
		if err := formula.Retire(s.clock.Now()); err != nil {
			return err
		}
		snapshot.Formulas[id] = formula
		updated = formula.Clone()
		return nil
	})
	if err != nil {
		return domain.Formula{}, err
	}
	return updated, nil
}

func (s *Service) SetFormulaTags(
	id string,
	input SetFormulaTagsInput,
) (domain.Formula, error) {
	var updated domain.Formula
	err := s.store.Execute(func(snapshot *domain.Snapshot) error {
		formula, ok := snapshot.Formulas[id]
		if !ok {
			return domain.NewError(domain.CodeFormulaNotFound, "formula was not found")
		}
		if err := s.checkExpected(formula.Version, input.ExpectedVersion); err != nil {
			return err
		}
		if err := formula.SetTags(input.Tags, s.clock.Now()); err != nil {
			return err
		}
		snapshot.Formulas[id] = formula
		updated = formula.Clone()
		return nil
	})
	if err != nil {
		return domain.Formula{}, err
	}
	return updated, nil
}

// RenameFormulaTag atomically renames one tag on every formula that carries
// it. The rename is guarded by the snapshot revision so two concurrent
// renames cannot silently interleave; the loser receives a revision conflict.
func (s *Service) RenameFormulaTag(input RenameTagInput) (RenameTagResult, error) {
	if input.ExpectedRevision == nil {
		return RenameTagResult{}, domain.FieldError(domain.CodeInvalidInput,
			"X-Expected-Version", "is required to rename a tag")
	}
	from := domain.NormalizeTag(input.From)
	if err := domain.ValidateTag(from, "from"); err != nil {
		return RenameTagResult{}, err
	}
	to := domain.NormalizeTag(input.To)
	if err := domain.ValidateTag(to, "to"); err != nil {
		return RenameTagResult{}, err
	}
	if from == to {
		return RenameTagResult{}, domain.FieldError(domain.CodeInvalidInput, "to",
			"must differ from the tag being renamed")
	}
	result := RenameTagResult{From: from, To: to}
	revision, err := s.store.Update(input.ExpectedRevision, func(snapshot *domain.Snapshot) error {
		carriers := 0
		occupied := false
		for _, formula := range snapshot.Formulas {
			if domain.HasTag(formula.Tags, from) {
				carriers++
			}
			if domain.HasTag(formula.Tags, to) {
				occupied = true
			}
		}
		if carriers == 0 {
			return domain.NewError(domain.CodeTagNotFound,
				"tag \""+from+"\" is not used by any formula")
		}
		if occupied {
			return domain.NewError(domain.CodeDuplicateTag,
				"tag \""+to+"\" is already used by another formula")
		}
		now := s.clock.Now()
		for id, formula := range snapshot.Formulas {
			if !domain.HasTag(formula.Tags, from) {
				continue
			}
			tags := make([]string, 0, len(formula.Tags))
			for _, tag := range formula.Tags {
				if tag == from {
					tags = append(tags, to)
				} else {
					tags = append(tags, tag)
				}
			}
			sort.Strings(tags)
			formula.Tags = tags
			formula.UpdatedAt = now.UTC()
			formula.Version++
			snapshot.Formulas[id] = formula
		}
		result.FormulasAffected = carriers
		return nil
	})
	if err != nil {
		return RenameTagResult{}, err
	}
	result.Revision = revision
	return result, nil
}

func (s *Service) GetFormula(id string) (domain.Formula, error) {
	var found domain.Formula
	err := s.store.View(func(snapshot domain.Snapshot) error {
		formula, ok := snapshot.Formulas[id]
		if !ok {
			return domain.NewError(domain.CodeFormulaNotFound, "formula was not found")
		}
		found = formula.Clone()
		return nil
	})
	if err != nil {
		return domain.Formula{}, err
	}
	return found, nil
}

// ListFormulas filters, sorts, and pages formulas from one consistent
// snapshot view, so the reported total always matches the page contents.
// Sorting is by normalized name with the identifier as a tie-breaker, which
// keeps pagination stable while tag maintenance only changes future queries.
func (s *Service) ListFormulas(query FormulaListQuery) (FormulaListResult, error) {
	filter, err := normalizeFormulaListQuery(query)
	if err != nil {
		return FormulaListResult{}, err
	}
	var matched []domain.Formula
	var revision int64
	err = s.store.View(func(snapshot domain.Snapshot) error {
		revision = snapshot.Revision
		matched = make([]domain.Formula, 0, len(snapshot.Formulas))
		for _, formula := range snapshot.Formulas {
			if !filter.matches(formula) {
				continue
			}
			matched = append(matched, formula.Clone())
		}
		return nil
	})
	if err != nil {
		return FormulaListResult{}, err
	}
	sort.Slice(matched, func(i, j int) bool {
		if matched[i].Name == matched[j].Name {
			return matched[i].ID < matched[j].ID
		}
		return matched[i].Name < matched[j].Name
	})
	result := FormulaListResult{
		Total:    len(matched),
		Revision: revision,
	}
	if filter.paged {
		start := (filter.page - 1) * filter.pageSize
		end := start + filter.pageSize
		if start > len(matched) {
			start = len(matched)
		}
		if end > len(matched) {
			end = len(matched)
		}
		result.Formulas = matched[start:end]
		result.Page = filter.page
		result.PageSize = filter.pageSize
	} else {
		result.Formulas = matched
	}
	if result.Formulas == nil {
		result.Formulas = []domain.Formula{}
	}
	return result, nil
}

// ListFormulaTags reports every tag in use with its formula count, sorted by
// tag, so production staff can pick filter values from a stable directory.
// The returned revision is the snapshot revision the directory was read from
// and the expected version for a guarded rename.
func (s *Service) ListFormulaTags() (TagDirectory, error) {
	counts := map[string]int{}
	var revision int64
	err := s.store.View(func(snapshot domain.Snapshot) error {
		revision = snapshot.Revision
		for _, formula := range snapshot.Formulas {
			for _, tag := range formula.Tags {
				counts[tag]++
			}
		}
		return nil
	})
	if err != nil {
		return TagDirectory{}, err
	}
	tags := make([]TagCount, 0, len(counts))
	for tag, count := range counts {
		tags = append(tags, TagCount{Tag: tag, Count: count})
	}
	sort.Slice(tags, func(i, j int) bool { return tags[i].Tag < tags[j].Tag })
	return TagDirectory{Tags: tags, Revision: revision}, nil
}

type formulaListFilter struct {
	nameKey   string
	styleTerm string
	state     domain.FormulaState
	tags      []string
	paged     bool
	page      int
	pageSize  int
}

func (f formulaListFilter) matches(formula domain.Formula) bool {
	if f.nameKey != "" && domain.NameKey(formula.Name) != f.nameKey {
		return false
	}
	if f.styleTerm != "" && !strings.Contains(domain.NameKey(formula.Style), f.styleTerm) {
		return false
	}
	if f.state != "" && formula.State != f.state {
		return false
	}
	for _, tag := range f.tags {
		if !domain.HasTag(formula.Tags, tag) {
			return false
		}
	}
	return true
}

func normalizeFormulaListQuery(query FormulaListQuery) (formulaListFilter, error) {
	filter := formulaListFilter{}
	if trimmed := domain.NormalizeName(query.Name); trimmed != "" {
		if len([]rune(trimmed)) > 120 {
			return filter, domain.FieldError(domain.CodeInvalidInput, "name",
				"must not exceed 120 characters")
		}
		filter.nameKey = domain.NameKey(trimmed)
	}
	if trimmed := domain.NormalizeName(query.Style); trimmed != "" {
		if len([]rune(trimmed)) > 80 {
			return filter, domain.FieldError(domain.CodeInvalidInput, "style",
				"must not exceed 80 characters")
		}
		// Style is free text, so filters use a case-insensitive substring
		// match; the name filter above stays an exact full-name match.
		filter.styleTerm = domain.NameKey(trimmed)
	}
	if query.State != "" {
		state := domain.FormulaState(query.State)
		switch state {
		case domain.FormulaDraft, domain.FormulaApproved, domain.FormulaRetired:
			filter.state = state
		default:
			return filter, domain.FieldError(domain.CodeInvalidInput, "state",
				"must be one of draft, approved, or retired")
		}
	}
	if len(query.Tags) > 0 {
		tags, err := domain.NormalizeTags(query.Tags)
		if err != nil {
			return filter, err
		}
		filter.tags = tags
	}
	if (query.Page == 0) != (query.PageSize == 0) {
		return filter, domain.NewError(domain.CodeInvalidPagination,
			"page and page_size must be supplied together")
	}
	if query.Page != 0 || query.PageSize != 0 {
		if query.Page < 1 || query.PageSize < 1 {
			return filter, domain.NewError(domain.CodeInvalidPagination,
				"page and page_size must be positive integers")
		}
		if query.PageSize > 200 {
			return filter, domain.NewError(domain.CodeInvalidPagination,
				"page_size must not exceed 200")
		}
		// Reject windows whose offset would overflow int; otherwise the
		// slice arithmetic below could panic or wrap negative.
		if query.Page-1 > (math.MaxInt-query.PageSize)/query.PageSize {
			return filter, domain.NewError(domain.CodeInvalidPagination,
				"page is too large to page this collection")
		}
		filter.paged = true
		filter.page = query.Page
		filter.pageSize = query.PageSize
	}
	return filter, nil
}

func timestampPointer(value time.Time) *time.Time {
	copy := value.UTC()
	return &copy
}
