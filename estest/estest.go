// Package estest simule un serveur Elasticsearch pour les tests unitaires des
// backends : un serveur HTTP local, un client réel branché dessus, et
// l'enregistrement des requêtes reçues.
package estest

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/elastic/go-elasticsearch/v8"
)

// Request est une requête reçue par le serveur simulé.
type Request struct {
	Method   string
	Path     string
	RawQuery string
	Body     []byte
}

// JSONBody décode le corps de la requête dans un objet générique.
func (r Request) JSONBody(t testing.TB) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(r.Body, &out); err != nil {
		t.Fatalf("corps non JSON (%s %s) : %v\n%s", r.Method, r.Path, err, r.Body)
	}
	return out
}

// Recorder garde la trace des requêtes reçues.
type Recorder struct {
	mu       sync.Mutex
	requests []Request
}

// Requests retourne une copie des requêtes reçues, dans l'ordre.
func (r *Recorder) Requests() []Request {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Request(nil), r.requests...)
}

// Find retourne les requêtes de la méthode et du chemin donnés.
func (r *Recorder) Find(method, path string) []Request {
	var found []Request
	for _, req := range r.Requests() {
		if req.Method == method && req.Path == path {
			found = append(found, req)
		}
	}
	return found
}

// JSON écrit une réponse JSON.
func JSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if body != nil {
		_ = json.NewEncoder(w).Encode(body)
	}
}

// New démarre un serveur simulé dont `handler` produit les réponses, et
// retourne un client Elasticsearch réel branché dessus. Le serveur est arrêté
// à la fin du test.
func New(t testing.TB, handler http.HandlerFunc) (*elasticsearch.Client, *Recorder) {
	t.Helper()
	recorder := &Recorder{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body))
		recorder.mu.Lock()
		recorder.requests = append(recorder.requests, Request{r.Method, r.URL.Path, r.URL.RawQuery, body})
		recorder.mu.Unlock()

		// Le client v8 refuse les serveurs qui ne s'annoncent pas comme Elasticsearch.
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		handler(w, r)
	}))
	t.Cleanup(server.Close)

	client, err := elasticsearch.NewClient(elasticsearch.Config{
		Addresses: []string{server.URL}, Username: "elastic", Password: "changeme",
	})
	if err != nil {
		t.Fatal(err)
	}
	return client, recorder
}
