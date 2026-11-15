package es

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/elastic/go-elasticsearch/v8"
)

// DataStreamTemplatePriority est la priorité des modèles d'index posés par
// EnsureDataStream : supérieure à celle des modèles intégrés d'Elasticsearch
// (`logs-*-*`, `metrics-*-*`, 100 à 200), qui ne doivent jamais l'emporter.
const DataStreamTemplatePriority = 500

// EnsureDataStream prépare le data stream `name` : modèle d'index composable
// (créé ou mis à jour), puis data stream (créé s'il n'existe pas).
//
// Le modèle ne cible que `name` exactement : il ne peut pas capter les index
// d'un autre projet. `mappings` doit déclarer `@timestamp` (type `date`),
// obligatoire dans un data stream.
//
// `retention` (ex. "730d") borne la durée de conservation via le cycle de vie
// natif des data streams ; vide, la conservation est illimitée (le cycle de vie
// reste actif pour gérer le rollover des index sous-jacents).
//
// Comme CreateIndex, la fonction est sans risque à chaque démarrage : si le
// data stream existe déjà, le mapping est fusionné (additif) sur son index
// d'écriture et la durée de conservation réappliquée.
//
// Retourne true si le data stream a été créé, false s'il existait déjà.
func EnsureDataStream(
	ctx context.Context, client *elasticsearch.Client, name string,
	mappings map[string]any, retention string,
) (bool, error) {
	lifecycle := map[string]any{}
	if retention != "" {
		lifecycle["data_retention"] = retention
	}

	template, err := jsonBody(map[string]any{
		"index_patterns": []string{name},
		"data_stream":    map[string]any{},
		"priority":       DataStreamTemplatePriority,
		"template":       map[string]any{"mappings": mappings, "lifecycle": lifecycle},
		"_meta":          map[string]any{"managed_by": "openhexa-core"},
	})
	if err != nil {
		return false, err
	}
	res, err := client.Indices.PutIndexTemplate(name, template, client.Indices.PutIndexTemplate.WithContext(ctx))
	if err != nil {
		return false, err
	}
	if err := checkResponse(res); err != nil {
		return false, err
	}
	res.Body.Close()

	exists, err := client.Indices.GetDataStream(
		client.Indices.GetDataStream.WithContext(ctx), client.Indices.GetDataStream.WithName(name))
	if err != nil {
		return false, err
	}
	exists.Body.Close()

	switch exists.StatusCode {
	case 404:
		res, err := client.Indices.CreateDataStream(name, client.Indices.CreateDataStream.WithContext(ctx))
		if err != nil {
			return false, err
		}
		if err := checkResponse(res); err != nil {
			return false, err
		}
		res.Body.Close()
		slog.Info("elasticsearch_data_stream_created", "data_stream", name, "retention", retention)
		return true, nil
	case 200:
		// Le modèle ne s'applique qu'aux futurs index sous-jacents : on
		// répercute le mapping et la conservation sur le data stream courant.
		body, err := jsonBody(mappings)
		if err != nil {
			return false, err
		}
		res, err := client.Indices.PutMapping([]string{name}, body, client.Indices.PutMapping.WithContext(ctx))
		if err != nil {
			return false, err
		}
		if err := checkResponse(res); err != nil {
			return false, err
		}
		res.Body.Close()

		body, err = jsonBody(lifecycle)
		if err != nil {
			return false, err
		}
		res, err = client.Indices.PutDataLifecycle([]string{name},
			client.Indices.PutDataLifecycle.WithContext(ctx), client.Indices.PutDataLifecycle.WithBody(body))
		if err != nil {
			return false, err
		}
		if err := checkResponse(res); err != nil {
			return false, err
		}
		res.Body.Close()
		return false, nil
	default:
		return false, fmt.Errorf("elasticsearch: GET _data_stream/%s a répondu %d", name, exists.StatusCode)
	}
}
