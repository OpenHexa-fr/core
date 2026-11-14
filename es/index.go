package es

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/elastic/go-elasticsearch/v8"
)

func jsonBody(v any) (*bytes.Reader, error) {
	payload, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(payload), nil
}

// CreateIndex crée l'index `name` avec le mapping explicite fourni s'il
// n'existe pas déjà.
//
// S'il existe déjà, les nouveaux champs du mapping y sont fusionnés (PUT
// mapping additif) : Elasticsearch ne recrée jamais les champs déjà présents,
// donc cette opération est sans risque et permet de faire évoluer le mapping
// explicite d'un domaine sans réindexation complète.
//
// Retourne true si l'index a été créé, false s'il existait déjà.
func CreateIndex(
	ctx context.Context, client *elasticsearch.Client, name string,
	mappings, settings map[string]any,
) (bool, error) {
	exists, err := client.Indices.Exists([]string{name}, client.Indices.Exists.WithContext(ctx))
	if err != nil {
		return false, err
	}
	exists.Body.Close()

	switch exists.StatusCode {
	case 200:
		body, err := jsonBody(mappings)
		if err != nil {
			return false, err
		}
		res, err := client.Indices.PutMapping([]string{name}, body, client.Indices.PutMapping.WithContext(ctx))
		if err != nil {
			return false, err
		}
		defer res.Body.Close()
		return false, checkResponse(res)
	case 404:
		if settings == nil {
			settings = map[string]any{}
		}
		body, err := jsonBody(map[string]any{"mappings": mappings, "settings": settings})
		if err != nil {
			return false, err
		}
		res, err := client.Indices.Create(name,
			client.Indices.Create.WithContext(ctx), client.Indices.Create.WithBody(body))
		if err != nil {
			return false, err
		}
		defer res.Body.Close()
		if err := checkResponse(res); err != nil {
			return false, err
		}
		slog.Info("elasticsearch_index_created", "index", name)
		return true, nil
	default:
		return false, fmt.Errorf("elasticsearch: HEAD /%s a répondu %d", name, exists.StatusCode)
	}
}

// EnsureAlias fait pointer `alias` vers `index`, en créant l'alias s'il
// n'existe pas déjà. Toute écriture des backends doit passer par l'alias,
// jamais par le nom d'index brut.
//
// Retourne true si l'alias a été créé, false s'il existait déjà.
func EnsureAlias(ctx context.Context, client *elasticsearch.Client, alias, index string) (bool, error) {
	exists, err := client.Indices.ExistsAlias([]string{alias},
		client.Indices.ExistsAlias.WithContext(ctx), client.Indices.ExistsAlias.WithIndex(index))
	if err != nil {
		return false, err
	}
	exists.Body.Close()

	switch exists.StatusCode {
	case 200:
		return false, nil
	case 404:
		res, err := client.Indices.PutAlias([]string{index}, alias, client.Indices.PutAlias.WithContext(ctx))
		if err != nil {
			return false, err
		}
		defer res.Body.Close()
		if err := checkResponse(res); err != nil {
			return false, err
		}
		slog.Info("elasticsearch_alias_created", "alias", alias, "index", index)
		return true, nil
	default:
		return false, fmt.Errorf("elasticsearch: HEAD alias %s a répondu %d", alias, exists.StatusCode)
	}
}

// SetupILM configure (crée ou met à jour) une politique ILM.
func SetupILM(ctx context.Context, client *elasticsearch.Client, policyName string, policy map[string]any) error {
	body, err := jsonBody(map[string]any{"policy": policy})
	if err != nil {
		return err
	}
	res, err := client.ILM.PutLifecycle(policyName,
		client.ILM.PutLifecycle.WithContext(ctx), client.ILM.PutLifecycle.WithBody(body))
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if err := checkResponse(res); err != nil {
		return err
	}
	slog.Info("elasticsearch_ilm_configured", "policy_name", policyName)
	return nil
}
