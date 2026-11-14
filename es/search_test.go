package es

import (
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"testing"

	"github.com/openhexa-fr/core/estest"
	"github.com/openhexa-fr/core/pagination"
)

func TestBuildFiltersIgnoresNil(t *testing.T) {
	var absent *string
	code := "13001"

	filters := BuildFilters(
		Filter{"commune", nil}, Filter{"absent", absent}, Filter{"code_postal", "13001"}, Filter{"present", &code},
	)

	want := []map[string]any{
		{"term": map[string]any{"code_postal": "13001"}},
		{"term": map[string]any{"present": "13001"}},
	}
	if !reflect.DeepEqual(filters, want) {
		t.Errorf("filtres : %v", filters)
	}
}

func TestBuildFiltersBuildsTermsForCollections(t *testing.T) {
	filters := BuildFilters(Filter{"type_local", []string{"Maison", "Appartement"}})

	want := []map[string]any{{"terms": map[string]any{"type_local": []string{"Maison", "Appartement"}}}}
	if !reflect.DeepEqual(filters, want) {
		t.Errorf("filtres : %v", filters)
	}
}

func TestBuildFiltersIgnoresEmptyCollection(t *testing.T) {
	if got := BuildFilters(Filter{"type_local", []string{}}, Filter{"autre", []int(nil)}); len(got) != 0 {
		t.Errorf("filtres : %v", got)
	}
}

func searchHandler(body any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { estest.JSON(w, 200, body) }
}

func TestPaginateRejectsSizeAboveResultWindow(t *testing.T) {
	client, _ := estest.New(t, searchHandler(nil))

	_, err := Paginate(t.Context(), client, PaginateParams{Index: "openhexa-dvf", Size: 1001})

	if !errors.Is(err, ErrInvalidSize) {
		t.Errorf("erreur %v, attendu ErrInvalidSize", err)
	}
	if _, err := Paginate(t.Context(), client, PaginateParams{Index: "i", Size: -1}); !errors.Is(err, ErrInvalidSize) {
		t.Errorf("size négatif : %v", err)
	}
}

func TestPaginateReturnsNextSearchAfterAndCursor(t *testing.T) {
	client, _ := estest.New(t, searchHandler(map[string]any{"hits": map[string]any{
		"hits":  []any{map[string]any{"_id": "1", "_source": map[string]any{}, "sort": []any{1704067200000, 12}}},
		"total": map[string]any{"value": 10000, "relation": "gte"},
	}}))

	page, err := Paginate(t.Context(), client, PaginateParams{
		Index: "openhexa-dvf", Sort: []map[string]any{{"date": "asc"}},
	})

	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 10000 || page.TotalRelation != "gte" {
		t.Errorf("total=%d relation=%s", page.Total, page.TotalRelation)
	}
	want := []any{json.Number("1704067200000"), json.Number("12")}
	if !reflect.DeepEqual(page.NextSearchAfter, want) {
		t.Errorf("next_search_after : %#v", page.NextSearchAfter)
	}
	decoded, err := pagination.DecodeCursor(*page.NextCursor)
	if err != nil || !reflect.DeepEqual(decoded, want) {
		t.Errorf("curseur : %#v (%v)", decoded, err)
	}
}

func TestPaginateDefaultsRelationToExact(t *testing.T) {
	client, _ := estest.New(t, searchHandler(map[string]any{"hits": map[string]any{
		"hits": []any{}, "total": map[string]any{"value": 0},
	}}))

	page, err := Paginate(t.Context(), client, PaginateParams{Index: "openhexa-dvf"})

	if err != nil {
		t.Fatal(err)
	}
	if page.TotalRelation != "eq" {
		t.Errorf("relation %q", page.TotalRelation)
	}
	// Une page vide n'a pas de curseur.
	if page.NextCursor != nil || page.NextSearchAfter != nil {
		t.Errorf("curseur sur une page vide : %v %v", page.NextCursor, page.NextSearchAfter)
	}
}

func TestPaginateBuildsRequestBody(t *testing.T) {
	client, recorder := estest.New(t, searchHandler(map[string]any{"hits": map[string]any{
		"hits": []any{}, "total": map[string]any{"value": 0},
	}}))
	query := map[string]any{"bool": map[string]any{"filter": []any{map[string]any{"term": map[string]any{"a": "b"}}}}}

	_, err := Paginate(t.Context(), client, PaginateParams{
		Index: "openhexa-dvf", Query: query, Sort: []map[string]any{{"date": "asc"}},
		SearchAfter: []any{"x", 3}, Size: 50, Source: []string{"id_mutation", "location"},
	})

	if err != nil {
		t.Fatal(err)
	}
	searches := recorder.Find("POST", "/openhexa-dvf/_search")
	if len(searches) != 1 {
		t.Fatalf("requêtes : %+v", recorder.Requests())
	}
	body := searches[0].JSONBody(t)
	checks := map[string]any{
		"size":         float64(50),
		"search_after": []any{"x", float64(3)},
		"_source":      []any{"id_mutation", "location"},
	}
	for key, want := range checks {
		if !reflect.DeepEqual(body[key], want) {
			t.Errorf("%s = %v, attendu %v", key, body[key], want)
		}
	}
}

func TestPaginateOmitsOptionalFieldsByDefault(t *testing.T) {
	client, recorder := estest.New(t, searchHandler(map[string]any{"hits": map[string]any{
		"hits": []any{}, "total": map[string]any{"value": 0},
	}}))

	if _, err := Paginate(t.Context(), client, PaginateParams{Index: "openhexa-dvf"}); err != nil {
		t.Fatal(err)
	}

	body := recorder.Requests()[0].JSONBody(t)
	for _, key := range []string{"_source", "search_after"} {
		if _, present := body[key]; present {
			t.Errorf("%s ne doit pas être envoyé par défaut", key)
		}
	}
	if body["size"] != float64(DefaultPageSize) {
		t.Errorf("size = %v", body["size"])
	}
	if !reflect.DeepEqual(body["query"], map[string]any{"match_all": map[string]any{}}) {
		t.Errorf("query = %v", body["query"])
	}
}

func TestPaginateSurfacesElasticsearchErrors(t *testing.T) {
	client, _ := estest.New(t, func(w http.ResponseWriter, r *http.Request) {
		estest.JSON(w, 400, map[string]any{"error": "bad request"})
	})

	_, err := Paginate(t.Context(), client, PaginateParams{Index: "idx"})

	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 400 {
		t.Errorf("erreur %v", err)
	}
}

func TestCountReturnsDocumentCount(t *testing.T) {
	client, recorder := estest.New(t, searchHandler(map[string]any{"count": 42}))

	n, err := Count(t.Context(), client, "openhexa-dvf")

	if err != nil || n != 42 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if len(recorder.Find("POST", "/openhexa-dvf/_count"))+len(recorder.Find("GET", "/openhexa-dvf/_count")) != 1 {
		t.Errorf("requêtes : %+v", recorder.Requests())
	}
}

func TestSearchAfterAllIteratesUntilEmptyPage(t *testing.T) {
	calls := 0
	client, recorder := estest.New(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		hits := []any{}
		if calls == 1 {
			hits = []any{map[string]any{"_id": "1", "_source": map[string]any{}, "sort": []any{"a", 1}}}
		}
		estest.JSON(w, 200, map[string]any{"hits": map[string]any{"hits": hits, "total": map[string]any{"value": 1}}})
	})

	var ids []string
	for hit, err := range SearchAfterAll(t.Context(), client, "openhexa-dvf", nil, "date", 0) {
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, hit.ID)
	}

	if !reflect.DeepEqual(ids, []string{"1"}) {
		t.Errorf("ids %v", ids)
	}
	requests := recorder.Requests()
	if len(requests) != 2 {
		t.Fatalf("%d requêtes", len(requests))
	}
	first := requests[0].JSONBody(t)
	wantSort := []any{map[string]any{"date": "asc"}, map[string]any{"_seq_no": "asc"}}
	if !reflect.DeepEqual(first["sort"], wantSort) {
		t.Errorf("sort = %v", first["sort"])
	}
	if !reflect.DeepEqual(requests[1].JSONBody(t)["search_after"], []any{"a", float64(1)}) {
		t.Errorf("la 2e page doit repartir du dernier tri : %s", requests[1].Body)
	}
}

func TestSearchAfterAllEmitsErrors(t *testing.T) {
	client, _ := estest.New(t, func(w http.ResponseWriter, r *http.Request) { estest.JSON(w, 500, nil) })

	var got error
	for _, err := range SearchAfterAll(t.Context(), client, "idx", nil, "date", 10) {
		got = err
	}

	if got == nil {
		t.Error("erreur attendue")
	}
}

func TestGetSource(t *testing.T) {
	client, _ := estest.New(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/openhexa-stations/_doc/known" {
			estest.JSON(w, 200, map[string]any{"_id": "known", "found": true, "_source": map[string]any{"station_id": "12345678"}})
			return
		}
		estest.JSON(w, 404, map[string]any{"_id": "unknown", "found": false})
	})

	source, err := GetSource(t.Context(), client, "openhexa-stations", "known")
	if err != nil || string(source) != `{"station_id":"12345678"}` {
		t.Errorf("source=%s err=%v", source, err)
	}

	if _, err := GetSource(t.Context(), client, "openhexa-stations", "unknown"); !errors.Is(err, ErrNotFound) {
		t.Errorf("erreur %v, attendu ErrNotFound", err)
	}
}

func TestDecodeHits(t *testing.T) {
	type doc struct {
		Name string `json:"name"`
	}
	hits := []Hit{{ID: "1", Source: json.RawMessage(`{"name":"a","ignored":1}`)}, {ID: "2", Source: json.RawMessage(`{"name":"b"}`)}}

	items, err := DecodeHits[doc](hits)

	if err != nil || !reflect.DeepEqual(items, []doc{{"a"}, {"b"}}) {
		t.Errorf("items=%v err=%v", items, err)
	}
	if _, err := DecodeHits[doc]([]Hit{{ID: "x", Source: json.RawMessage(`[1]`)}}); err == nil {
		t.Error("erreur attendue sur un `_source` illisible")
	}
}
