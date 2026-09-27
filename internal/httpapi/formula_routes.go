package httpapi

import (
	"net/http"
	"strconv"

	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/domain"
	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/service"
)

type createFormulaRequest struct {
	Name                  string   `json:"name"`
	Style                 string   `json:"style"`
	TargetOriginalGravity float64  `json:"target_original_gravity"`
	TargetFinalGravity    float64  `json:"target_final_gravity"`
	MaxFermentationTempC  float64  `json:"max_fermentation_temp_c"`
	MinimumDays           int      `json:"minimum_days"`
	MaximumDays           int      `json:"maximum_days"`
	Tags                  []string `json:"tags"`
}

type setStepsRequest struct {
	Steps []domain.FormulaStep `json:"steps"`
}

type setTagsRequest struct {
	Tags []string `json:"tags"`
}

type renameTagRequest struct {
	From string `json:"from"`
	To   string `json:"to"`
}

func registerFormulaRoutes(mux *http.ServeMux, app *service.Service) {
	mux.HandleFunc("POST /v1/formulas", func(response http.ResponseWriter, request *http.Request) {
		body, err := decodeJSON[createFormulaRequest](response, request)
		if !handleDecode(response, err) {
			return
		}
		formula, err := app.CreateFormula(service.CreateFormulaInput{
			Name:                  body.Name,
			Style:                 body.Style,
			TargetOriginalGravity: body.TargetOriginalGravity,
			TargetFinalGravity:    body.TargetFinalGravity,
			MaxFermentationTempC:  body.MaxFermentationTempC,
			MinimumDays:           body.MinimumDays,
			MaximumDays:           body.MaximumDays,
			Tags:                  body.Tags,
		})
		if err != nil {
			writeDomainError(response, err)
			return
		}
		writeJSON(response, http.StatusCreated, formula)
	})

	mux.HandleFunc("GET /v1/formulas", func(response http.ResponseWriter, request *http.Request) {
		query, err := formulaListQuery(request)
		if !handleDecode(response, err) {
			return
		}
		result, err := app.ListFormulas(query)
		if err != nil {
			writeDomainError(response, err)
			return
		}
		writeJSON(response, http.StatusOK, map[string]any{
			"formulas":  result.Formulas,
			"total":     result.Total,
			"page":      result.Page,
			"page_size": result.PageSize,
			"revision":  result.Revision,
		})
	})

	mux.HandleFunc("GET /v1/formulas/{id}", func(response http.ResponseWriter, request *http.Request) {
		formula, err := app.GetFormula(request.PathValue("id"))
		if err != nil {
			writeDomainError(response, err)
			return
		}
		writeJSON(response, http.StatusOK, formula)
	})

	mux.HandleFunc("PUT /v1/formulas/{id}/steps", func(response http.ResponseWriter, request *http.Request) {
		body, err := decodeJSON[setStepsRequest](response, request)
		if !handleDecode(response, err) {
			return
		}
		expected, err := expectedVersion(request)
		if !handleDecode(response, err) {
			return
		}
		formula, err := app.SetFormulaSteps(request.PathValue("id"),
			service.SetFormulaStepsInput{ExpectedVersion: expected, Steps: body.Steps})
		if err != nil {
			writeDomainError(response, err)
			return
		}
		writeJSON(response, http.StatusOK, formula)
	})

	mux.HandleFunc("PUT /v1/formulas/{id}/tags", func(response http.ResponseWriter, request *http.Request) {
		body, err := decodeJSON[setTagsRequest](response, request)
		if !handleDecode(response, err) {
			return
		}
		expected, err := expectedVersion(request)
		if !handleDecode(response, err) {
			return
		}
		formula, err := app.SetFormulaTags(request.PathValue("id"),
			service.SetFormulaTagsInput{ExpectedVersion: expected, Tags: body.Tags})
		if err != nil {
			writeDomainError(response, err)
			return
		}
		writeJSON(response, http.StatusOK, formula)
	})

	mux.HandleFunc("POST /v1/formulas/{id}/approve", func(response http.ResponseWriter, request *http.Request) {
		expected, err := expectedVersion(request)
		if !handleDecode(response, err) {
			return
		}
		formula, err := app.ApproveFormula(request.PathValue("id"),
			service.VersionInput{ExpectedVersion: expected})
		if err != nil {
			writeDomainError(response, err)
			return
		}
		writeJSON(response, http.StatusOK, formula)
	})

	mux.HandleFunc("POST /v1/formulas/{id}/retire", func(response http.ResponseWriter, request *http.Request) {
		expected, err := expectedVersion(request)
		if !handleDecode(response, err) {
			return
		}
		formula, err := app.RetireFormula(request.PathValue("id"),
			service.VersionInput{ExpectedVersion: expected})
		if err != nil {
			writeDomainError(response, err)
			return
		}
		writeJSON(response, http.StatusOK, formula)
	})

	mux.HandleFunc("GET /v1/tags", func(response http.ResponseWriter, _ *http.Request) {
		directory, err := app.ListFormulaTags()
		if err != nil {
			writeDomainError(response, err)
			return
		}
		writeJSON(response, http.StatusOK, map[string]any{
			"tags":     directory.Tags,
			"revision": directory.Revision,
		})
	})

	mux.HandleFunc("POST /v1/tags/rename", func(response http.ResponseWriter, request *http.Request) {
		body, err := decodeJSON[renameTagRequest](response, request)
		if !handleDecode(response, err) {
			return
		}
		expected, err := expectedVersion(request)
		if !handleDecode(response, err) {
			return
		}
		result, err := app.RenameFormulaTag(service.RenameTagInput{
			ExpectedRevision: expected,
			From:             body.From,
			To:               body.To,
		})
		if err != nil {
			writeDomainError(response, err)
			return
		}
		writeJSON(response, http.StatusOK, map[string]any{
			"from":              result.From,
			"to":                result.To,
			"formulas_affected": result.FormulasAffected,
			"revision":          result.Revision,
		})
	})
}

// formulaListQuery parses the list filters and pagination window. Pagination
// is all-or-nothing: page and page_size must appear together, and any invalid
// value is rejected instead of silently widening the window.
func formulaListQuery(request *http.Request) (service.FormulaListQuery, error) {
	values := request.URL.Query()
	query := service.FormulaListQuery{
		Name:  values.Get("name"),
		Style: values.Get("style"),
		State: values.Get("state"),
		Tags:  values["tag"],
	}
	pageRaw := values.Get("page")
	sizeRaw := values.Get("page_size")
	if (pageRaw == "") != (sizeRaw == "") {
		missing := "page_size"
		if sizeRaw == "" {
			missing = "page"
		}
		return service.FormulaListQuery{}, domain.FieldError(domain.CodeInvalidPagination,
			missing, "page and page_size must be supplied together")
	}
	if pageRaw == "" && sizeRaw == "" {
		return query, nil
	}
	page, err := parsePageParameter("page", pageRaw)
	if err != nil {
		return service.FormulaListQuery{}, err
	}
	size, err := parsePageParameter("page_size", sizeRaw)
	if err != nil {
		return service.FormulaListQuery{}, err
	}
	query.Page = page
	query.PageSize = size
	return query, nil
}

func parsePageParameter(field, raw string) (int, error) {
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed < 1 {
		return 0, domain.FieldError(domain.CodeInvalidPagination, field,
			"must be a positive integer")
	}
	return parsed, nil
}
