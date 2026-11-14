package models

import (
	"encoding/json"
	"testing"

	"github.com/openhexa-fr/core/es"
)

func TestFromPageSerializesLikeThePythonResponse(t *testing.T) {
	cursor := "abc"
	page := es.Page{
		Hits:          []es.Hit{{ID: "1", Source: json.RawMessage(`{"n":1}`)}},
		Total:         10000,
		TotalRelation: "gte",
		NextCursor:    &cursor,
	}

	response, err := FromPage[struct {
		N int `json:"n"`
	}](page)
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(response)

	want := `{"items":[{"n":1}],"total":10000,"total_relation":"gte","next_cursor":"abc","next_search_after":null}`
	if string(payload) != want {
		t.Errorf("\n got %s\nwant %s", payload, want)
	}
}

func TestFromPageEmptyPageHasEmptyItemsAndNullCursor(t *testing.T) {
	response, err := FromPage[map[string]any](es.Page{TotalRelation: "eq"})
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(response)

	want := `{"items":[],"total":0,"total_relation":"eq","next_cursor":null,"next_search_after":null}`
	if string(payload) != want {
		t.Errorf("\n got %s\nwant %s", payload, want)
	}
}
