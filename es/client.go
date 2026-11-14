// Package es est la couche d'accès à Elasticsearch partagée par les backends
// OpenHexa : client avec retry, gestion des index et alias, ingestion bulk
// idempotente et recherche paginée.
//
// Elasticsearch reste volontairement manipulé en JSON brut (map[string]any) :
// les requêtes des domaines métier (scripts Painless, agrégations composite,
// geo_distance...) sont des documents JSON, et un client typé n'apporterait ici
// que de la verbosité.
package es

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/elastic/go-elasticsearch/v8"

	"github.com/openhexa-fr/core/config"
)

// ErrMissingCredentials est retournée quand ni api key ni user/password ne sont fournis.
var ErrMissingCredentials = errors.New("es: ES_API_KEY, ou ES_USER et ES_PASSWORD, est obligatoire")

// ConnectOptions règle le retry de connexion.
type ConnectOptions struct {
	// MaxRetries est le nombre total de tentatives de ping (défaut 15).
	MaxRetries int
	// BaseDelay est le délai avant la 2e tentative, doublé à chaque échec (défaut 1 s).
	BaseDelay time.Duration
	// MaxDelay plafonne le délai entre deux tentatives (défaut 10 s).
	MaxDelay time.Duration
}

// Budget total par défaut ~100 s : un cluster ES conteneurisé peut rester
// injoignable un moment après que son propre healthcheck Docker le déclare
// "healthy".
func (o ConnectOptions) withDefaults() ConnectOptions {
	if o.MaxRetries <= 0 {
		o.MaxRetries = 15
	}
	if o.BaseDelay <= 0 {
		o.BaseDelay = time.Second
	}
	if o.MaxDelay <= 0 {
		o.MaxDelay = 10 * time.Second
	}
	return o
}

// NewClient construit le client ES, en préférant l'API key (accès scopé) au
// user/password. Aucune requête n'est envoyée : voir Connect.
func NewClient(settings config.ESSettings) (*elasticsearch.Client, error) {
	cfg := elasticsearch.Config{Addresses: []string{settings.URL}}
	switch {
	case settings.APIKey != "":
		cfg.APIKey = settings.APIKey
	case settings.User != "" && settings.Password != "":
		cfg.Username = settings.User
		cfg.Password = settings.Password
	default:
		return nil, ErrMissingCredentials
	}
	return elasticsearch.NewClient(cfg)
}

// Connect construit le client et attend que le cluster réponde, avec un backoff
// exponentiel (utile au démarrage d'un conteneur avant qu'ES ne soit prêt).
func Connect(ctx context.Context, settings config.ESSettings) (*elasticsearch.Client, error) {
	return ConnectWithOptions(ctx, settings, ConnectOptions{})
}

// ConnectWithOptions est Connect avec un réglage explicite du retry.
func ConnectWithOptions(
	ctx context.Context, settings config.ESSettings, options ConnectOptions,
) (*elasticsearch.Client, error) {
	options = options.withDefaults()
	client, err := NewClient(settings)
	if err != nil {
		return nil, err
	}

	delay := options.BaseDelay
	for attempt := 1; ; attempt++ {
		if HealthCheck(ctx, client) {
			return client, nil
		}
		if attempt >= options.MaxRetries {
			return nil, fmt.Errorf("elasticsearch unreachable at %s after %d attempts", settings.URL, options.MaxRetries)
		}
		slog.Warn("elasticsearch_unreachable_retry", "attempt", attempt, "delay", delay)

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
		delay = min(delay*2, options.MaxDelay)
	}
}

// HealthCheck vérifie que le cluster Elasticsearch répond.
func HealthCheck(ctx context.Context, client *elasticsearch.Client) bool {
	res, err := client.Ping(client.Ping.WithContext(ctx))
	if err != nil {
		// Toute erreur réseau vaut "down".
		return false
	}
	defer res.Body.Close()
	return !res.IsError()
}
