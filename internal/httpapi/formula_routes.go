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
	Tags                  []string `json:"tags"`
	TargetOriginalGravity float64  `json:"target_original_gravity"`
	TargetFinalGravity    float64  `json:"target_final_gravity"`
	MaxFermentationTempC  float64  `json:"max_fermentation_temp_c"`
	MinimumDays           int      `json:"minimum_days"`
	MaximumDays           int      `json:"maximum_days"`
}

type setStepsRequest struct {
	Steps []domain.FormulaStep `json:"steps"`
}

type setTagsRequest struct {
	Tags []string `json:"tags"`
}

func parseFormulaFilter(request *http.Request) (service.FormulaFilter, error) {
	query := request.URL.Query()
	filter := service.FormulaFilter{
		Name:   query.Get("name"),
		Style:  query.Get("style"),
		State:  query.Get("state"),
		Tags:   query["tag"],
		Limit:  service.DefaultFormulaListLimit,
		Offset: 0,
	}
	if raw := query.Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			return filter, domain.FieldError(domain.CodeInvalidInput, "limit",
				"must be an integer")
		}
		filter.Limit = parsed
	}
	if raw := query.Get("offset"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			return filter, domain.FieldError(domain.CodeInvalidInput, "offset",
				"must be an integer")
		}
		filter.Offset = parsed
	}
	return filter, nil
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
			Tags:                  body.Tags,
			TargetOriginalGravity: body.TargetOriginalGravity,
			TargetFinalGravity:    body.TargetFinalGravity,
			MaxFermentationTempC:  body.MaxFermentationTempC,
			MinimumDays:           body.MinimumDays,
			MaximumDays:           body.MaximumDays,
		})
		if err != nil {
			writeDomainError(response, err)
			return
		}
		writeJSON(response, http.StatusCreated, formula)
	})

	mux.HandleFunc("GET /v1/formulas", func(response http.ResponseWriter, request *http.Request) {
		filter, err := parseFormulaFilter(request)
		if !handleDecode(response, err) {
			return
		}
		page, err := app.ListFormulas(filter)
		if err != nil {
			writeDomainError(response, err)
			return
		}
		writeJSON(response, http.StatusOK, page)
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
}
