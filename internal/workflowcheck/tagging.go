package workflowcheck

import (
	"fmt"
	"strings"

	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/domain"
)

func formulaTagging(api *client) error {
	alpha, err := createTaggedFormula(api, "Tag Alpha", "India Pale", []string{"pilot", "Citra"})
	if err != nil {
		return err
	}
	if len(alpha.Tags) != 2 || alpha.Tags[0] != "pilot" || alpha.Tags[1] != "Citra" {
		return fmt.Errorf("created formula did not echo normalized tags: %v", alpha.Tags)
	}
	bravo, err := createTaggedFormula(api, "Tag Bravo", "India Pale", []string{"pilot"})
	if err != nil {
		return err
	}
	if _, err := createTaggedFormula(api, "Tag Charlie", "Cellar Lager", nil); err != nil {
		return err
	}

	duplicateCreate, err := api.call("POST", "/v1/formulas",
		createFormulaTaggedPayload("Tag Delta", "India Pale", []string{"Pilot", "pilot"}), nil)
	if err != nil {
		return err
	}
	if err := requireCode(duplicateCreate, 400, string(domain.CodeDuplicateTag),
		"create formula with duplicate tags"); err != nil {
		return err
	}

	if err := requireListTotal(api, "/v1/formulas?tag=pilot", 2, "filter by one tag"); err != nil {
		return err
	}
	combined, err := listFormulas(api, "/v1/formulas?tag=PILOT&tag=citra")
	if err != nil {
		return err
	}
	if combined.Total != 1 || len(combined.Formulas) != 1 ||
		combined.Formulas[0].ID != alpha.ID {
		return fmt.Errorf("tag combination filter returned %+v", combined)
	}
	if err := requireListTotal(api, "/v1/formulas?style=india%20pale", 2,
		"filter by style"); err != nil {
		return err
	}
	byName, err := listFormulas(api, "/v1/formulas?name=tag%20alpha")
	if err != nil {
		return err
	}
	if byName.Total != 1 || byName.Formulas[0].ID != alpha.ID {
		return fmt.Errorf("full name filter returned %+v", byName)
	}
	if err := requireListTotal(api, "/v1/formulas?state=draft", 3,
		"filter by state"); err != nil {
		return err
	}

	firstPage, err := listFormulas(api, "/v1/formulas?limit=2&offset=0")
	if err != nil {
		return err
	}
	secondPage, err := listFormulas(api, "/v1/formulas?limit=2&offset=2")
	if err != nil {
		return err
	}
	if firstPage.Total != 3 || secondPage.Total != 3 {
		return fmt.Errorf("paged totals diverged: %d and %d",
			firstPage.Total, secondPage.Total)
	}
	if len(firstPage.Formulas) != 2 || len(secondPage.Formulas) != 1 {
		return fmt.Errorf("paged results have unexpected sizes")
	}
	ordered := []string{
		firstPage.Formulas[0].Name,
		firstPage.Formulas[1].Name,
		secondPage.Formulas[0].Name,
	}
	if strings.Join(ordered, ",") != "Tag Alpha,Tag Bravo,Tag Charlie" {
		return fmt.Errorf("paged ordering is not stable: %v", ordered)
	}
	if firstPage.Formulas[0].ID == secondPage.Formulas[0].ID ||
		firstPage.Formulas[1].ID == secondPage.Formulas[0].ID {
		return fmt.Errorf("paged results overlap")
	}

	for _, query := range []string{
		"/v1/formulas?limit=0",
		"/v1/formulas?limit=201",
		"/v1/formulas?limit=abc",
		"/v1/formulas?offset=-1",
		"/v1/formulas?offset=two",
		"/v1/formulas?state=bogus",
	} {
		invalid, err := api.call("GET", query, nil, nil)
		if err != nil {
			return err
		}
		if err := requireCode(invalid, 400, string(domain.CodeInvalidInput),
			"reject invalid list query "+query); err != nil {
			return err
		}
	}

	duplicateTags, err := api.call("PUT", "/v1/formulas/"+alpha.ID+"/tags",
		map[string]any{"tags": []string{"Citra", "citra"}}, nil)
	if err != nil {
		return err
	}
	if err := requireCode(duplicateTags, 400, string(domain.CodeDuplicateTag),
		"replace tags with duplicates"); err != nil {
		return err
	}

	retagged, err := api.call("PUT", "/v1/formulas/"+alpha.ID+"/tags",
		map[string]any{"tags": []string{"dry-hop", "pilot"}}, ptrInt64(1))
	if err != nil {
		return err
	}
	if err := requireStatus(retagged, 200, "replace tags"); err != nil {
		return err
	}
	if err := retagged.decode(&alpha); err != nil {
		return err
	}
	if alpha.Version != 2 || len(alpha.Tags) != 2 || alpha.Tags[0] != "dry-hop" {
		return fmt.Errorf("tag replacement returned %+v", alpha)
	}

	stale, err := api.call("PUT", "/v1/formulas/"+alpha.ID+"/tags",
		map[string]any{"tags": []string{"other"}}, ptrInt64(1))
	if err != nil {
		return err
	}
	if err := requireCode(stale, 412, string(domain.CodeRevisionConflict),
		"replace tags with a stale version"); err != nil {
		return err
	}

	stepsResponse, err := api.call("PUT", "/v1/formulas/"+alpha.ID+"/steps",
		formulaStepPayload(), ptrInt64(2))
	if err != nil {
		return err
	}
	if err := requireStatus(stepsResponse, 200, "set steps on tagged formula"); err != nil {
		return err
	}
	approvedResponse, err := api.call("POST", "/v1/formulas/"+alpha.ID+"/approve",
		nil, ptrInt64(3))
	if err != nil {
		return err
	}
	if err := requireStatus(approvedResponse, 200, "approve tagged formula"); err != nil {
		return err
	}

	runResponse, err := api.call("POST", "/v1/runs",
		createRunPayload("RUN-TAGGING-A", alpha.ID), nil)
	if err != nil {
		return err
	}
	if err := requireStatus(runResponse, 201, "create run for tagged formula"); err != nil {
		return err
	}
	var taggedRun runView
	if err := runResponse.decode(&taggedRun); err != nil {
		return err
	}

	approvedRetag, err := api.call("PUT", "/v1/formulas/"+alpha.ID+"/tags",
		map[string]any{"tags": []string{"approved-pilot"}}, ptrInt64(4))
	if err != nil {
		return err
	}
	if err := requireStatus(approvedRetag, 200, "retag approved formula"); err != nil {
		return err
	}
	if err := approvedRetag.decode(&alpha); err != nil {
		return err
	}
	if alpha.State != domain.FormulaApproved || len(alpha.Steps) != 3 ||
		len(alpha.Tags) != 1 || alpha.Tags[0] != "approved-pilot" {
		return fmt.Errorf("retagging changed approved process content: %+v", alpha)
	}

	runReadback, err := api.call("GET", "/v1/runs/"+taggedRun.ID, nil, nil)
	if err != nil {
		return err
	}
	if err := requireStatus(runReadback, 200, "read run after retag"); err != nil {
		return err
	}
	if err := runReadback.decode(&taggedRun); err != nil {
		return err
	}
	if taggedRun.FormulaID != alpha.ID {
		return fmt.Errorf("retagging changed the run formula reference")
	}

	if err := requireListTotal(api, "/v1/formulas?tag=approved-pilot", 1,
		"query reflects new tags"); err != nil {
		return err
	}
	if err := requireListTotal(api, "/v1/formulas?tag=dry-hop", 0,
		"query no longer matches replaced tags"); err != nil {
		return err
	}

	detail, err := api.call("GET", "/v1/formulas/"+bravo.ID, nil, nil)
	if err != nil {
		return err
	}
	if err := requireStatus(detail, 200, "read tagged formula detail"); err != nil {
		return err
	}
	var bravoDetail formulaView
	if err := detail.decode(&bravoDetail); err != nil {
		return err
	}
	if len(bravoDetail.Tags) != 1 || bravoDetail.Tags[0] != "pilot" {
		return fmt.Errorf("formula detail did not include tags: %v", bravoDetail.Tags)
	}
	return nil
}

func createTaggedFormula(
	api *client,
	name, style string,
	tags []string,
) (formulaView, error) {
	created, err := api.call("POST", "/v1/formulas",
		createFormulaTaggedPayload(name, style, tags), nil)
	if err != nil {
		return formulaView{}, err
	}
	if err := requireStatus(created, 201, "create tagged formula"); err != nil {
		return formulaView{}, err
	}
	var formula formulaView
	if err := created.decode(&formula); err != nil {
		return formulaView{}, err
	}
	return formula, nil
}

func listFormulas(api *client, query string) (formulaListView, error) {
	listed, err := api.call("GET", query, nil, nil)
	if err != nil {
		return formulaListView{}, err
	}
	if err := requireStatus(listed, 200, "list formulas"); err != nil {
		return formulaListView{}, err
	}
	var page formulaListView
	if err := listed.decode(&page); err != nil {
		return formulaListView{}, err
	}
	if len(page.Formulas) > page.Total {
		return formulaListView{}, fmt.Errorf(
			"page holds %d entries but total is %d", len(page.Formulas), page.Total)
	}
	return page, nil
}

func requireListTotal(api *client, query string, total int, operation string) error {
	page, err := listFormulas(api, query)
	if err != nil {
		return err
	}
	if page.Total != total {
		return fmt.Errorf("%s: expected total %d, got %d", operation, total, page.Total)
	}
	return nil
}
