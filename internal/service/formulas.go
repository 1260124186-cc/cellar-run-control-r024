package service

import (
	"sort"
	"time"

	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/domain"
)

const (
	DefaultFormulaListLimit = 50
	MaxFormulaListLimit     = 200
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
		Tags:                  tags,
		TargetOriginalGravity: input.TargetOriginalGravity,
		TargetFinalGravity:    input.TargetFinalGravity,
		MaxFermentationTempC:  input.MaxFermentationTempC,
		MinimumDays:           input.MinimumDays,
		MaximumDays:           input.MaximumDays,
		Steps:                 []domain.FormulaStep{},
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

func (s *Service) ListFormulas(filter FormulaFilter) (FormulaPage, error) {
	if filter.Limit < 1 || filter.Limit > MaxFormulaListLimit {
		return FormulaPage{}, domain.FieldError(domain.CodeInvalidInput, "limit",
			"must be between 1 and 200")
	}
	if filter.Offset < 0 {
		return FormulaPage{}, domain.FieldError(domain.CodeInvalidInput, "offset",
			"must not be negative")
	}
	var stateFilter domain.FormulaState
	if filter.State != "" {
		switch domain.FormulaState(filter.State) {
		case domain.FormulaDraft, domain.FormulaApproved, domain.FormulaRetired:
			stateFilter = domain.FormulaState(filter.State)
		default:
			return FormulaPage{}, domain.FieldError(domain.CodeInvalidInput, "state",
				"must be one of draft, approved, retired")
		}
	}
	tagKeys := make(map[string]struct{}, len(filter.Tags))
	for _, raw := range filter.Tags {
		tag := domain.NormalizeTag(raw)
		if tag == "" {
			return FormulaPage{}, domain.FieldError(domain.CodeInvalidInput, "tag",
				"must not be empty")
		}
		tagKeys[domain.TagKey(tag)] = struct{}{}
	}
	nameKey := ""
	if filter.Name != "" {
		nameKey = domain.NameKey(filter.Name)
	}
	styleKey := ""
	if filter.Style != "" {
		styleKey = domain.NameKey(filter.Style)
	}

	var matched []domain.Formula
	err := s.store.View(func(snapshot domain.Snapshot) error {
		matched = make([]domain.Formula, 0, len(snapshot.Formulas))
		for _, formula := range snapshot.Formulas {
			if !formulaMatches(formula, nameKey, styleKey, stateFilter, tagKeys) {
				continue
			}
			matched = append(matched, formula.Clone())
		}
		return nil
	})
	if err != nil {
		return FormulaPage{}, err
	}
	sort.Slice(matched, func(i, j int) bool {
		if matched[i].Name == matched[j].Name {
			return matched[i].ID < matched[j].ID
		}
		return matched[i].Name < matched[j].Name
	})
	total := len(matched)
	page := make([]domain.Formula, 0, min(filter.Limit, total))
	if filter.Offset < total {
		end := min(filter.Offset+filter.Limit, total)
		page = append(page, matched[filter.Offset:end]...)
	}
	return FormulaPage{
		Formulas: page,
		Total:    total,
		Limit:    filter.Limit,
		Offset:   filter.Offset,
	}, nil
}

func formulaMatches(
	formula domain.Formula,
	nameKey string,
	styleKey string,
	state domain.FormulaState,
	tagKeys map[string]struct{},
) bool {
	if nameKey != "" && domain.NameKey(formula.Name) != nameKey {
		return false
	}
	if styleKey != "" && domain.NameKey(formula.Style) != styleKey {
		return false
	}
	if state != "" && formula.State != state {
		return false
	}
	if len(tagKeys) > 0 {
		held := make(map[string]struct{}, len(formula.Tags))
		for _, tag := range formula.Tags {
			held[domain.TagKey(tag)] = struct{}{}
		}
		for key := range tagKeys {
			if _, ok := held[key]; !ok {
				return false
			}
		}
	}
	return true
}

func timestampPointer(value time.Time) *time.Time {
	copy := value.UTC()
	return &copy
}
