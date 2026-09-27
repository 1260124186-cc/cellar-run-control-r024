package workflowcheck

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/domain"
)

// formulaTagManagement exercises the tag maintenance and query workflow that
// production staff rely on once the formula list grows: filter by style,
// state, and tag combination, page stably by full name, maintain tags even
// after approval without altering process content, and reject duplicate
// tags, invalid pagination, and stale renames.
func formulaTagManagement(api *client) error {
	// Create one approved pilot (tags may be attached at creation) and one
	// draft so state filtering has something to distinguish.
	pilot, err := createTaggedFormula(api, "Pilot Saison", []string{"summer", "belgian"},
		"Rustic Saison")
	if err != nil {
		return err
	}
	pilot, err = approveCheckFormula(api, pilot.ID, pilot.Version)
	if err != nil {
		return err
	}

	if _, err := createTaggedFormula(api, "Pilot Stout", []string{"winter", "dark"},
		"Dark Stout"); err != nil {
		return err
	}
	third, err := createTaggedFormula(api, "Pilot Wheat", []string{"summer", "wheat"},
		"Cloudy Wheat")
	if err != nil {
		return err
	}

	// Tags are maintainable on an approved formula, and maintenance bumps
	// the version but must not touch process steps.
	setResponse, err := api.call("PUT", "/v1/formulas/"+pilot.ID+"/tags",
		map[string]any{"tags": []string{"summer", "belgian", "pilot"}}, ptrInt64(pilot.Version))
	if err != nil {
		return err
	}
	if err := requireStatus(setResponse, 200, "set tags on approved formula"); err != nil {
		return err
	}
	var tagged formulaView
	if err := setResponse.decode(&tagged); err != nil {
		return err
	}
	if tagged.Version != pilot.Version+1 || len(tagged.Tags) != 3 {
		return errInvalidWorkflow("approved formula tag update did not return the expected revision")
	}
	if len(tagged.Steps) != 3 || tagged.TargetOriginalGravity != 1.052 {
		return errInvalidWorkflow("tag maintenance changed approved process content")
	}

	// Duplicate tags after normalization are a conflict, not a silent drop.
	duplicate, err := api.call("PUT", "/v1/formulas/"+third.ID+"/tags",
		map[string]any{"tags": []string{"summer", " Summer "}}, ptrInt64(1))
	if err != nil {
		return err
	}
	if err := requireCode(duplicate, 409, string(domain.CodeDuplicateTag),
		"set duplicate tags"); err != nil {
		return err
	}

	// Combined filters: state=approved + tag=summer narrows to the pilot.
	filtered, err := listFormulas(api, url.Values{
		"state": {"approved"},
		"tag":   {"summer"},
	})
	if err != nil {
		return err
	}
	if filtered.Total != 1 || len(filtered.Formulas) != 1 ||
		filtered.Formulas[0].ID != pilot.ID {
		return errInvalidWorkflow("state + tag filter returned the wrong formulas")
	}

	// A tag combination is an AND across tags.
	both, err := listFormulas(api, url.Values{
		"tag": {"summer", "wheat"},
	})
	if err != nil {
		return err
	}
	if both.Total != 1 || both.Formulas[0].ID != third.ID {
		return errInvalidWorkflow("tag combination filter was not evaluated as AND")
	}

	// Style filter and full-name filter combine with the other filters.
	styled, err := listFormulas(api, url.Values{"style": {"stout"}})
	if err != nil {
		return err
	}
	if styled.Total != 1 || styled.Formulas[0].Name != "Pilot Stout" {
		return errInvalidWorkflow("style filter returned the wrong formula")
	}
	named, err := listFormulas(api, url.Values{"name": {"  pilot   saison "}})
	if err != nil {
		return err
	}
	if named.Total != 1 || named.Formulas[0].ID != pilot.ID {
		return errInvalidWorkflow("full-name filter did not normalize exactly")
	}

	// Pagination: total counts the whole filtered set while the page carries
	// the window; pages concatenate back to the unpaged order with no gaps.
	if err := verifyPagination(api); err != nil {
		return err
	}

	// Invalid pagination windows fail with a stable code instead of silently
	// widening the result.
	for name, query := range map[string]url.Values{
		"page alone":     {"page": {"1"}},
		"size alone":     {"page_size": {"2"}},
		"zero page":      {"page": {"0"}, "page_size": {"2"}},
		"non-numeric":    {"page": {"two"}, "page_size": {"2"}},
		"oversized size": {"page": {"1"}, "page_size": {"9999"}},
		"page past int":  {"page": {"99999999999999999999"}, "page_size": {"2"}},
	} {
		response, err := api.call("GET", "/v1/formulas?"+query.Encode(), nil, nil)
		if err != nil {
			return err
		}
		if err := requireCode(response, 400, string(domain.CodeInvalidPagination),
			"reject "+name); err != nil {
			return err
		}
	}

	// Tag directory reports the revision needed to guard a rename.
	directoryResponse, err := api.call("GET", "/v1/tags", nil, nil)
	if err != nil {
		return err
	}
	if err := requireStatus(directoryResponse, 200, "list tag directory"); err != nil {
		return err
	}
	var directory struct {
		Tags     []tagCountView `json:"tags"`
		Revision int64          `json:"revision"`
	}
	if err := directoryResponse.decode(&directory); err != nil {
		return err
	}
	darkCount := -1
	for _, entry := range directory.Tags {
		if entry.Tag == "dark" {
			darkCount = entry.Count
		}
	}
	if darkCount != 1 {
		return errInvalidWorkflow("tag directory did not report expected tag counts")
	}

	// Rename the "dark" tag, guarded by the directory revision.
	renameResponse, err := api.call("POST", "/v1/tags/rename",
		map[string]any{"from": "dark", "to": "roast"}, ptrInt64(directory.Revision))
	if err != nil {
		return err
	}
	if err := requireStatus(renameResponse, 200, "rename tag"); err != nil {
		return err
	}
	var rename struct {
		From             string `json:"from"`
		To               string `json:"to"`
		FormulasAffected int    `json:"formulas_affected"`
		Revision         int64  `json:"revision"`
	}
	if err := renameResponse.decode(&rename); err != nil {
		return err
	}
	if rename.From != "dark" || rename.To != "roast" ||
		rename.FormulasAffected != 1 || rename.Revision != directory.Revision+1 {
		return errInvalidWorkflow("rename response did not describe the applied change")
	}

	// A concurrent rename based on the stale directory revision conflicts.
	stale, err := api.call("POST", "/v1/tags/rename",
		map[string]any{"from": "winter", "to": "cold-season"}, ptrInt64(directory.Revision))
	if err != nil {
		return err
	}
	if err := requireCode(stale, 412, string(domain.CodeRevisionConflict),
		"concurrent rename"); err != nil {
		return err
	}

	// Renaming onto an existing tag and renaming an unknown tag are explicit
	// failures.
	collision, err := api.call("POST", "/v1/tags/rename",
		map[string]any{"from": "winter", "to": "summer"}, ptrInt64(rename.Revision))
	if err != nil {
		return err
	}
	if err := requireCode(collision, 409, string(domain.CodeDuplicateTag),
		"rename onto existing tag"); err != nil {
		return err
	}
	missing, err := api.call("POST", "/v1/tags/rename",
		map[string]any{"from": "ghost", "to": "phantom"}, ptrInt64(rename.Revision))
	if err != nil {
		return err
	}
	if err := requireCode(missing, 404, string(domain.CodeTagNotFound),
		"rename missing tag"); err != nil {
		return err
	}
	unguarded, err := api.call("POST", "/v1/tags/rename",
		map[string]any{"from": "winter", "to": "cold"}, nil)
	if err != nil {
		return err
	}
	if err := requireCode(unguarded, 400, string(domain.CodeInvalidInput),
		"rename without expected revision"); err != nil {
		return err
	}

	// After the rename, new queries use the new tag while the run reference
	// (formula id) is untouched: the renamed carrier is found under "roast".
	renamed, err := listFormulas(api, url.Values{"tag": {"roast"}})
	if err != nil {
		return err
	}
	if renamed.Total != 1 || renamed.Formulas[0].Name != "Pilot Stout" {
		return errInvalidWorkflow("renamed tag did not affect subsequent queries")
	}
	gone, err := listFormulas(api, url.Values{"tag": {"dark"}})
	if err != nil {
		return err
	}
	if gone.Total != 0 {
		return errInvalidWorkflow("old tag name still matched after rename")
	}
	return nil
}

func verifyPagination(api *client) error {
	full, err := listFormulas(api, url.Values{})
	if err != nil {
		return err
	}
	if full.Total != len(full.Formulas) || full.Page != 0 || full.PageSize != 0 {
		return errInvalidWorkflow("unpaged list reported inconsistent totals")
	}
	lastNonEmpty := (full.Total + 1) / 2
	if full.Total%2 == 1 {
		lastNonEmpty++
	}
	var seen []string
	for page := 1; page <= lastNonEmpty+1; page++ {
		query := url.Values{"page": {strconv.Itoa(page)}, "page_size": {"2"}}
		window, err := listFormulas(api, query)
		if err != nil {
			return err
		}
		if window.Total != full.Total || window.Page != page || window.PageSize != 2 {
			return errInvalidWorkflow("paged list total drifted from the filtered set")
		}
		if page == lastNonEmpty+1 && len(window.Formulas) != 0 {
			return errInvalidWorkflow("page beyond the filtered set was not empty")
		}
		for _, formula := range window.Formulas {
			seen = append(seen, formula.ID)
		}
	}
	if len(seen) != full.Total {
		return errInvalidWorkflow("pages did not concatenate to the full result set")
	}
	for index, formula := range full.Formulas {
		if formula.ID != seen[index] {
			return errInvalidWorkflow("page order did not match the stable name order")
		}
	}
	return nil
}

func createTaggedFormula(api *client, name string, tags []string, style string) (formulaView, error) {
	payload := createFormulaPayload(name)
	payload["style"] = style
	payload["tags"] = tags
	response, err := api.call("POST", "/v1/formulas", payload, nil)
	if err != nil {
		return formulaView{}, err
	}
	if err := requireStatus(response, 201, "create tagged formula"); err != nil {
		return formulaView{}, err
	}
	var formula formulaView
	if err := response.decode(&formula); err != nil {
		return formulaView{}, err
	}
	if len(formula.Tags) != len(tags) {
		return formulaView{}, fmt.Errorf("created formula did not store %d tags", len(tags))
	}
	return formula, nil
}

func approveCheckFormula(api *client, id string, draftVersion int64) (formulaView, error) {
	steps, err := api.call("PUT", "/v1/formulas/"+id+"/steps",
		formulaStepPayload(), ptrInt64(draftVersion))
	if err != nil {
		return formulaView{}, err
	}
	if err := requireStatus(steps, 200, "set check formula steps"); err != nil {
		return formulaView{}, err
	}
	var withSteps formulaView
	if err := steps.decode(&withSteps); err != nil {
		return formulaView{}, err
	}
	approved, err := api.call("POST", "/v1/formulas/"+id+"/approve",
		nil, ptrInt64(withSteps.Version))
	if err != nil {
		return formulaView{}, err
	}
	if err := requireStatus(approved, 200, "approve check formula"); err != nil {
		return formulaView{}, err
	}
	var formula formulaView
	if err := approved.decode(&formula); err != nil {
		return formulaView{}, err
	}
	return formula, nil
}

func listFormulas(api *client, query url.Values) (formulaListView, error) {
	path := "/v1/formulas"
	if encoded := query.Encode(); encoded != "" {
		path += "?" + encoded
	}
	response, err := api.call("GET", path, nil, nil)
	if err != nil {
		return formulaListView{}, err
	}
	if err := requireStatus(response, 200, "list formulas"); err != nil {
		return formulaListView{}, err
	}
	var result formulaListView
	if err := response.decode(&result); err != nil {
		return formulaListView{}, err
	}
	if strings.Contains(path, "page=") && len(result.Formulas) > result.PageSize {
		return result, errInvalidWorkflow("page returned more rows than page_size")
	}
	return result, nil
}
