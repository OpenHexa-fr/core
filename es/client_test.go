package es

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openhexa-fr/core/config"
)

func TestNewClientRequiresCredentials(t *testing.T) {
	_, err := NewClient(config.ESSettings{URL: "http://localhost:9200"})
	if !errors.Is(err, ErrMissingCredentials) {
		t.Errorf("erreur %v, attendu ErrMissingCredentials", err)
	}
}

func TestNewClientAcceptsAPIKeyOrBasicAuth(t *testing.T) {
	for name, settings := range map[string]config.ESSettings{
		"api key":  {URL: "http://localhost:9200", APIKey: "dGVzdDp0ZXN0"},
		"basic":    {URL: "http://localhost:9200", User: "elastic", Password: "changeme"},
		"les deux": {URL: "http://localhost:9200", APIKey: "dGVzdDp0ZXN0", User: "elastic", Password: "changeme"},
	} {
		if _, err := NewClient(settings); err != nil {
			t.Errorf("%s : %v", name, err)
		}
	}
}

// Le serveur reçoit l'en-tête d'autorisation : l'API key est préférée au user/password.
func TestNewClientPrefersAPIKeyOverBasicAuth(t *testing.T) {
	var authorization atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization.Store(r.Header.Get("Authorization"))
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		w.WriteHeader(200)
	}))
	defer server.Close()

	client, err := NewClient(config.ESSettings{
		URL: server.URL, APIKey: "dGVzdDp0ZXN0", User: "elastic", Password: "changeme",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !HealthCheck(t.Context(), client) {
		t.Fatal("ping en échec")
	}
	if got := authorization.Load(); got != "APIKey dGVzdDp0ZXN0" {
		t.Errorf("Authorization = %v", got)
	}
}

func TestHealthCheck(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		w.WriteHeader(200)
	}))
	defer up.Close()
	client, _ := NewClient(config.ESSettings{URL: up.URL, User: "u", Password: "p"})
	if !HealthCheck(t.Context(), client) {
		t.Error("cluster joignable : HealthCheck doit retourner true")
	}

	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := down.URL
	down.Close()
	client, _ = NewClient(config.ESSettings{URL: url, User: "u", Password: "p"})
	if HealthCheck(t.Context(), client) {
		t.Error("cluster injoignable : HealthCheck doit retourner false")
	}
}

func TestConnectRetriesUntilClusterIsUp(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		// Les 2 premiers pings échouent : un cluster pas encore prêt.
		if calls.Add(1) <= 2 {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(200)
	}))
	defer server.Close()

	client, err := ConnectWithOptions(t.Context(),
		config.ESSettings{URL: server.URL, User: "u", Password: "p"},
		ConnectOptions{MaxRetries: 5, BaseDelay: time.Millisecond, MaxDelay: 2 * time.Millisecond})

	if err != nil || client == nil {
		t.Fatalf("client=%v err=%v", client, err)
	}
}

func TestConnectFailsAfterMaxRetries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
	}))
	defer server.Close()

	_, err := ConnectWithOptions(t.Context(),
		config.ESSettings{URL: server.URL, User: "u", Password: "p"},
		ConnectOptions{MaxRetries: 3, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond})

	if err == nil {
		t.Error("erreur attendue après épuisement des tentatives")
	}
}

func TestConnectStopsWhenContextIsCancelled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(t.Context())

	done := make(chan error, 1)
	go func() {
		_, err := ConnectWithOptions(ctx,
			config.ESSettings{URL: server.URL, User: "u", Password: "p"},
			ConnectOptions{MaxRetries: 100, BaseDelay: time.Hour, MaxDelay: time.Hour})
		done <- err
	}()

	select {
	case <-done:
		t.Fatal("ne devrait pas se terminer avant l'annulation")
	case <-time.After(50 * time.Millisecond):
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("erreur %v, attendu context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Connect ne s'arrête pas à l'annulation du contexte")
	}
}
