package es

import (
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/openhexa-fr/core/estest"
)

const historyStream = "openhexa-essence-prix-historique"

var historyMapping = map[string]any{"properties": map[string]any{"@timestamp": Date()}}

func TestEnsureDataStreamCreatesTemplateThenStream(t *testing.T) {
	client, recorder := estest.New(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			estest.JSON(w, 404, map[string]any{"error": "absent"})
			return
		}
		estest.JSON(w, 200, map[string]any{"acknowledged": true})
	})

	created, err := EnsureDataStream(t.Context(), client, historyStream, historyMapping, "730d")

	if err != nil || !created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	templates := recorder.Find("PUT", "/_index_template/"+historyStream)
	if len(templates) != 1 {
		t.Fatalf("%d modèles posés : %+v", len(templates), recorder.Requests())
	}
	body := templates[0].JSONBody(t)
	if !reflect.DeepEqual(body["index_patterns"], []any{historyStream}) {
		t.Errorf("index_patterns doit cibler le seul data stream : %v", body["index_patterns"])
	}
	if _, ok := body["data_stream"]; !ok {
		t.Error("le modèle doit déclarer data_stream")
	}
	template := body["template"].(map[string]any)
	if !reflect.DeepEqual(template["lifecycle"], map[string]any{"data_retention": "730d"}) {
		t.Errorf("lifecycle : %v", template["lifecycle"])
	}
	if len(recorder.Find("PUT", "/_data_stream/"+historyStream)) != 1 {
		t.Error("le data stream doit être créé")
	}
	if len(recorder.Find("PUT", "/"+historyStream+"/_mapping")) != 0 {
		t.Error("pas de put_mapping sur un data stream fraîchement créé")
	}
}

func TestEnsureDataStreamUpdatesExistingStream(t *testing.T) {
	client, recorder := estest.New(t, status(200))

	created, err := EnsureDataStream(t.Context(), client, historyStream, historyMapping, "")

	if err != nil || created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	if len(recorder.Find("PUT", "/_data_stream/"+historyStream)) != 0 {
		t.Error("un data stream existant ne doit pas être recréé")
	}
	if len(recorder.Find("PUT", "/"+historyStream+"/_mapping")) != 1 {
		t.Error("le mapping doit être fusionné sur le data stream existant")
	}
	lifecycle := recorder.Find("PUT", "/_data_stream/"+historyStream+"/_lifecycle")
	if len(lifecycle) != 1 {
		t.Fatalf("la conservation doit être réappliquée : %+v", recorder.Requests())
	}
	if body := lifecycle[0].JSONBody(t); len(body) != 0 {
		t.Errorf("sans retention, conservation illimitée attendue : %v", body)
	}
}

func TestEnsureDataStreamReportsTemplateFailure(t *testing.T) {
	client, recorder := estest.New(t, func(w http.ResponseWriter, r *http.Request) {
		estest.JSON(w, 403, map[string]any{"error": "security_exception"})
	})

	_, err := EnsureDataStream(t.Context(), client, historyStream, historyMapping, "")

	if err == nil || !strings.Contains(err.Error(), "security_exception") {
		t.Errorf("erreur attendue, reçu %v", err)
	}
	if len(recorder.Requests()) != 1 {
		t.Error("rien d'autre ne doit être tenté si le modèle est refusé")
	}
}

func TestBulkCreateCountsConflictsAsDuplicates(t *testing.T) {
	// L'indexer répartit les documents entre plusieurs workers, donc plusieurs
	// requêtes : la réponse est construite action par action.
	statuses := map[string]int{"a": 201, "b": 409, "c": 400}
	client, recorder := estest.New(t, func(w http.ResponseWriter, r *http.Request) {
		var items []any
		for line := range strings.SplitSeq(strings.TrimSpace(string(mustRead(t, r))), "\n") {
			var action map[string]map[string]any
			if json.Unmarshal([]byte(line), &action) != nil || action["create"] == nil {
				continue
			}
			id := action["create"]["_id"].(string)
			item := map[string]any{"_id": id, "status": statuses[id]}
			if statuses[id] >= 300 {
				item["error"] = map[string]any{"type": "erreur"}
			}
			items = append(items, map[string]any{"create": item})
		}
		estest.JSON(w, 200, map[string]any{"errors": true, "items": items})
	})
	docs := []Document{{"_id": "a"}, {"_id": "b"}, {"_id": "c"}}

	created, duplicates, failed, err := BulkCreate(t.Context(), client, historyStream, slices.Values(docs))

	if err != nil || created != 1 || duplicates != 1 || failed != 1 {
		t.Errorf("created=%d duplicates=%d failed=%d err=%v", created, duplicates, failed, err)
	}
	for _, bulk := range recorder.Find("POST", "/_bulk") {
		body := string(bulk.Body)
		if !strings.Contains(body, `{"create":{`) || !strings.Contains(body, `"_index":"`+historyStream+`"`) {
			t.Errorf("action create attendue : %s", bulk.Body)
		}
	}
}

func TestExistingIDsReturnsKnownIDs(t *testing.T) {
	client, recorder := estest.New(t, func(w http.ResponseWriter, r *http.Request) {
		estest.JSON(w, 200, map[string]any{"hits": map[string]any{"hits": []any{
			map[string]any{"_id": "b"},
		}}})
	})

	existing, err := ExistingIDs(t.Context(), client, historyStream, []string{"a", "b"})

	if err != nil || !reflect.DeepEqual(existing, map[string]bool{"b": true}) {
		t.Fatalf("existing=%v err=%v", existing, err)
	}
	searches := recorder.Find("POST", "/"+historyStream+"/_search")
	if len(searches) != 1 {
		t.Fatalf("%+v", recorder.Requests())
	}
	if !strings.Contains(searches[0].RawQuery, "ignore_unavailable=true") {
		t.Errorf("un data stream absent ne doit pas être une erreur : %s", searches[0].RawQuery)
	}
	query := searches[0].JSONBody(t)["query"].(map[string]any)
	if !reflect.DeepEqual(query, map[string]any{"ids": map[string]any{"values": []any{"a", "b"}}}) {
		t.Errorf("query : %v", query)
	}
}

func TestExistingIDsChunksLargeRequests(t *testing.T) {
	client, recorder := estest.New(t, func(w http.ResponseWriter, r *http.Request) {
		estest.JSON(w, 200, map[string]any{"hits": map[string]any{"hits": []any{}}})
	})
	ids := make([]string, existingIDsChunk+1)
	for i := range ids {
		ids[i] = string(rune('a' + i%26))
	}

	if _, err := ExistingIDs(t.Context(), client, historyStream, ids); err != nil {
		t.Fatal(err)
	}
	if n := len(recorder.Find("POST", "/"+historyStream+"/_search")); n != 2 {
		t.Errorf("%d requêtes, 2 attendues", n)
	}
}

func TestExistingIDsWithoutIDsDoesNothing(t *testing.T) {
	client, recorder := estest.New(t, status(200))

	existing, err := ExistingIDs(t.Context(), client, historyStream, nil)

	if err != nil || len(existing) != 0 || len(recorder.Requests()) != 0 {
		t.Errorf("existing=%v err=%v requêtes=%d", existing, err, len(recorder.Requests()))
	}
}

func mustRead(t *testing.T, r *http.Request) []byte {
	t.Helper()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatal(err)
	}
	return body
}
