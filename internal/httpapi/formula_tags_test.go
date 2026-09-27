package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/clock"
	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/domain"
	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/service"
	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/storage"
)

func newTagTestHandler(t *testing.T) (http.Handler, *service.Service) {
	t.Helper()
	store, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	app := service.New(store, clock.NewFixed(time.Date(2026, 9, 27, 8, 0, 0, 0, time.UTC)))
	return NewHandler(app), app
}

func createFormulaOverHTTP(t *testing.T, handler http.Handler, name string, tags []string) map[string]any {
	t.Helper()
	payload, _ := json.Marshal(map[string]any{
		"name":                    name,
		"style":                   "Pale",
		"target_original_gravity": 1.05,
		"target_final_gravity":    1.01,
		"max_fermentation_temp_c": 20,
		"minimum_days":            5,
		"maximum_days":            10,
		"tags":                    tags,
	})
	request := httptest.NewRequest(http.MethodPost, "/v1/formulas", bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("create formula: status %d body %s", response.Code, response.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created formula: %v", err)
	}
	return created
}

func TestListPaginationErrors(t *testing.T) {
	handler, _ := newTagTestHandler(t)
	cases := map[string]string{
		"page only":    "/v1/formulas?page=1",
		"size only":    "/v1/formulas?page_size=10",
		"zero page":    "/v1/formulas?page=0&page_size=10",
		"text page":    "/v1/formulas?page=abc&page_size=10",
		"size too big": "/v1/formulas?page=1&page_size=500",
	}
	for name, target := range cases {
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, target, nil)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", response.Code, response.Body.String())
			}
			var body struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode error: %v", err)
			}
			if body.Error.Code != string(domain.CodeInvalidPagination) {
				t.Fatalf("expected invalid_pagination, got %q", body.Error.Code)
			}
		})
	}
}

func TestSetTagsDuplicateConflict(t *testing.T) {
	handler, _ := newTagTestHandler(t)
	created := createFormulaOverHTTP(t, handler, "Tagged Formula", []string{"summer"})
	id := created["id"].(string)

	payload := strings.NewReader(`{"tags":["Summer","summer"]}`)
	request := httptest.NewRequest(http.MethodPut, "/v1/formulas/"+id+"/tags", payload)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", response.Code, response.Body.String())
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error.Code != string(domain.CodeDuplicateTag) {
		t.Fatalf("expected duplicate_tag, got %q", body.Error.Code)
	}
}

func TestPagedTotalMatchesPages(t *testing.T) {
	handler, _ := newTagTestHandler(t)
	for index := 0; index < 5; index++ {
		createFormulaOverHTTP(t, handler,
			"Formula "+string(rune('a'+index)), []string{"batch"})
	}
	ids := map[string]bool{}
	var total int
	for page := 1; page <= 3; page++ {
		request := httptest.NewRequest(http.MethodGet,
			"/v1/formulas?tag=batch&page="+strconv.Itoa(page)+"&page_size=2", nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("page %d: %s", page, response.Body.String())
		}
		var result struct {
			Formulas []map[string]any `json:"formulas"`
			Total    int              `json:"total"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatalf("decode: %v", err)
		}
		total = result.Total
		for _, formula := range result.Formulas {
			id := formula["id"].(string)
			if ids[id] {
				t.Fatalf("formula %s appeared on two pages", id)
			}
			ids[id] = true
		}
	}
	if total != 5 || len(ids) != 5 {
		t.Fatalf("total %d did not match unique rows %d", total, len(ids))
	}
}

func TestRenameRequiresExpectedVersion(t *testing.T) {
	handler, _ := newTagTestHandler(t)
	createFormulaOverHTTP(t, handler, "Renameable", []string{"old"})

	payload := strings.NewReader(`{"from":"old","to":"new"}`)
	request := httptest.NewRequest(http.MethodPost, "/v1/tags/rename", payload)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 without expected version, got %d", response.Code)
	}
}
