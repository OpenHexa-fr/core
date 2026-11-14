package es

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/openhexa-fr/core/estest"
)

func status(code int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		estest.JSON(w, code, map[string]any{"acknowledged": true})
	}
}

func TestCreateIndexCreatesWhenAbsent(t *testing.T) {
	client, recorder := estest.New(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			w.WriteHeader(404)
			return
		}
		estest.JSON(w, 200, map[string]any{"acknowledged": true})
	})
	mappings := map[string]any{"properties": map[string]any{"id": Keyword()}}

	created, err := CreateIndex(t.Context(), client, "openhexa-dvf-000001", mappings, nil)

	if err != nil || !created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	puts := recorder.Find("PUT", "/openhexa-dvf-000001")
	if len(puts) != 1 {
		t.Fatalf("%d créations", len(puts))
	}
	body := puts[0].JSONBody(t)
	if !reflect.DeepEqual(body["mappings"], map[string]any{"properties": map[string]any{"id": map[string]any{"type": "keyword"}}}) {
		t.Errorf("mappings : %v", body["mappings"])
	}
	if !reflect.DeepEqual(body["settings"], map[string]any{}) {
		t.Errorf("settings : %v", body["settings"])
	}
	if len(recorder.Find("PUT", "/openhexa-dvf-000001/_mapping")) != 0 {
		t.Error("pas de put_mapping quand l'index est créé")
	}
}

func TestCreateIndexMergesNewFieldsWhenPresent(t *testing.T) {
	client, recorder := estest.New(t, func(w http.ResponseWriter, r *http.Request) {
		estest.JSON(w, 200, map[string]any{"acknowledged": true})
	})
	mappings := map[string]any{"properties": map[string]any{"prix_m2": Float()}}

	created, err := CreateIndex(t.Context(), client, "openhexa-dvf-000001", mappings, nil)

	if err != nil || created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	puts := recorder.Find("PUT", "/openhexa-dvf-000001/_mapping")
	if len(puts) != 1 {
		t.Fatalf("%d put_mapping", len(puts))
	}
	if !reflect.DeepEqual(puts[0].JSONBody(t), map[string]any{"properties": map[string]any{"prix_m2": map[string]any{"type": "float"}}}) {
		t.Errorf("corps : %s", puts[0].Body)
	}
	if len(recorder.Find("PUT", "/openhexa-dvf-000001")) != 0 {
		t.Error("l'index existant ne doit pas être recréé")
	}
}

func TestCreateIndexPropagatesErrors(t *testing.T) {
	client, _ := estest.New(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			w.WriteHeader(404)
			return
		}
		estest.JSON(w, 400, map[string]any{"error": "mapping invalide"})
	})

	if _, err := CreateIndex(t.Context(), client, "idx", map[string]any{}, nil); err == nil {
		t.Error("erreur attendue")
	}
}

func TestEnsureAliasCreatesWhenAbsent(t *testing.T) {
	client, recorder := estest.New(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			w.WriteHeader(404)
			return
		}
		estest.JSON(w, 200, map[string]any{"acknowledged": true})
	})

	created, err := EnsureAlias(t.Context(), client, "openhexa-dvf", "openhexa-dvf-000001")

	if err != nil || !created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	if len(recorder.Find("PUT", "/openhexa-dvf-000001/_aliases/openhexa-dvf")) != 1 {
		t.Errorf("requêtes : %+v", recorder.Requests())
	}
}

func TestEnsureAliasNoopWhenPresent(t *testing.T) {
	client, recorder := estest.New(t, status(200))

	created, err := EnsureAlias(t.Context(), client, "openhexa-dvf", "openhexa-dvf-000001")

	if err != nil || created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	for _, req := range recorder.Requests() {
		if req.Method == "PUT" {
			t.Errorf("écriture inattendue : %s %s", req.Method, req.Path)
		}
	}
}

func TestSetupILMPutsPolicy(t *testing.T) {
	client, recorder := estest.New(t, status(200))

	err := SetupILM(t.Context(), client, "openhexa-policy", map[string]any{"phases": map[string]any{}})

	if err != nil {
		t.Fatal(err)
	}
	puts := recorder.Find("PUT", "/_ilm/policy/openhexa-policy")
	if len(puts) != 1 {
		t.Fatalf("requêtes : %+v", recorder.Requests())
	}
	if _, ok := puts[0].JSONBody(t)["policy"]; !ok {
		t.Errorf("corps : %s", puts[0].Body)
	}
}
