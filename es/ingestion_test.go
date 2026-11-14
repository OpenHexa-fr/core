package es

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/openhexa-fr/core/estest"
)

func TestMakeDocumentIDIsDeterministic(t *testing.T) {
	first := MakeDocumentID("2024-01-01", "13202", "001")
	second := MakeDocumentID("2024-01-01", "13202", "001")

	if first != second || len(first) != 16 {
		t.Errorf("ids %q / %q", first, second)
	}
	if MakeDocumentID("a", "b") == MakeDocumentID("a", "c") {
		t.Error("des parts différentes doivent donner des ids différents")
	}
}

// Valeurs de référence calculées avec l'implémentation Python
// (sha256("|".join(str(p) for p in parts))[:16]) : les `_id` des documents déjà
// indexés doivent rester identiques, sans quoi une ré-ingestion crée des doublons.
func TestMakeDocumentIDMatchesPythonImplementation(t *testing.T) {
	tests := []struct {
		name  string
		parts []any
		want  string
	}{
		{"station", []any{"12345678"}, "ef797c8118f02dfb"},
		{"trois chaînes", []any{"2024-01-01", "13202", "001"}, "b082488126339fbd"},
		{"deux chaînes", []any{"a", "b"}, "0eab8a0a3380abf4"},
		{"dvf avec occurrence entière", []any{"mut1", "1", "AB0001", "1", "12", 0}, "6480ae6ce58680ec"},
		{"None et entier", []any{"mut1", "", "", "", nil, 3}, "70728cc1e827d413"},
		{
			"flottants, booléens et entiers",
			[]any{1.0, 100.0, 0.0001, 1e-5, 1e16, 123456789012345680.0, 2500.75, true, false, 12, -3.5},
			"99734447268a1443",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := MakeDocumentID(tc.parts...); got != tc.want {
				t.Errorf("MakeDocumentID = %s, attendu %s", got, tc.want)
			}
		})
	}
}

func TestPyFloatMatchesPythonRepr(t *testing.T) {
	tests := map[float64]string{
		1.0: "1.0", 100.0: "100.0", 0.0001: "0.0001", 1e-5: "1e-05", 1e16: "1e+16",
		123456789012345680.0: "1.2345678901234568e+17", 2500.75: "2500.75", -3.5: "-3.5",
		0.1: "0.1", 1.0 / 3: "0.3333333333333333", 1e22: "1e+22", 5e-324: "5e-324",
		12345.678: "12345.678", 1e15: "1000000000000000.0", 9999999999999998.0: "9999999999999998.0",
	}
	for value, want := range tests {
		if got := pyFloat(value); got != want {
			t.Errorf("pyFloat(%v) = %q, attendu %q", value, got, want)
		}
	}
}

func TestBulkIndexWritesViaAlias(t *testing.T) {
	client, recorder := estest.New(t, func(w http.ResponseWriter, r *http.Request) {
		estest.JSON(w, 200, map[string]any{
			"errors": false,
			"items":  []any{map[string]any{"index": map[string]any{"_id": "abc123", "status": 201}}},
		})
	})
	docs := []Document{{"_id": "abc123", "nom": "Test"}}

	success, failed, err := BulkIndex(t.Context(), client, "openhexa-dvf", slices.Values(docs))

	if err != nil || success != 1 || failed != 0 {
		t.Fatalf("success=%d failed=%d err=%v", success, failed, err)
	}
	bulk := recorder.Find("POST", "/_bulk")
	if len(bulk) != 1 {
		t.Fatalf("%d appels _bulk", len(bulk))
	}
	lines := strings.Split(strings.TrimSpace(string(bulk[0].Body)), "\n")
	if len(lines) != 2 {
		t.Fatalf("corps bulk : %q", bulk[0].Body)
	}
	var action map[string]map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &action); err != nil {
		t.Fatal(err)
	}
	if action["index"]["_index"] != "openhexa-dvf" || action["index"]["_id"] != "abc123" {
		t.Errorf("action : %v", action)
	}
	if lines[1] != `{"nom":"Test"}` {
		t.Errorf("le `_source` ne doit pas contenir `_id` : %s", lines[1])
	}
	if docs[0]["_id"] != "abc123" {
		t.Error("le document de l'appelant ne doit pas être modifié")
	}
}

func TestBulkIndexReportsErrorCount(t *testing.T) {
	client, _ := estest.New(t, func(w http.ResponseWriter, r *http.Request) {
		estest.JSON(w, 200, map[string]any{
			"errors": true,
			"items": []any{map[string]any{"index": map[string]any{
				"_id": "abc123", "status": 400,
				"error": map[string]any{"type": "mapper_parsing_exception", "reason": "boom"},
			}}},
		})
	})
	docs := []Document{{"_id": "abc123", "nom": "Test"}}

	success, failed, err := BulkIndex(t.Context(), client, "openhexa-dvf", slices.Values(docs))

	if err != nil || success != 0 || failed != 1 {
		t.Fatalf("success=%d failed=%d err=%v", success, failed, err)
	}
}

func TestBulkIndexRejectsDocumentWithoutID(t *testing.T) {
	client, _ := estest.New(t, func(w http.ResponseWriter, r *http.Request) {
		estest.JSON(w, 200, map[string]any{"errors": false, "items": []any{}})
	})

	_, _, err := BulkIndex(t.Context(), client, "idx", slices.Values([]Document{{"nom": "sans id"}}))

	if err == nil || !strings.Contains(err.Error(), "_id") {
		t.Errorf("erreur attendue sur l'absence de `_id`, reçu %v", err)
	}
}

func TestBulkIndexReportsTransportFailure(t *testing.T) {
	client, _ := estest.New(t, func(w http.ResponseWriter, r *http.Request) {
		estest.JSON(w, 500, map[string]any{"error": "boom"})
	})

	success, failed, err := BulkIndex(t.Context(), client, "idx", slices.Values([]Document{{"_id": "a"}}))

	if err == nil || success != 0 || failed != 1 {
		t.Errorf("success=%d failed=%d err=%v", success, failed, err)
	}
}

func TestBulkIndexReportsACancelledContextOnce(t *testing.T) {
	client, _ := estest.New(t, func(w http.ResponseWriter, r *http.Request) {
		estest.JSON(w, 200, map[string]any{"errors": false, "items": []any{}})
	})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, _, err := BulkIndex(ctx, client, "idx", slices.Values([]Document{{"_id": "a"}}))

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("erreur %v, attendu context.Canceled", err)
	}
	if strings.Count(err.Error(), "context canceled") != 1 {
		t.Errorf("l'erreur ne doit pas être dupliquée : %q", err)
	}
}
