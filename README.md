# openhexa-core

Couche partagée d'accès à Elasticsearch pour les backends OpenHexa (`immo-back`, `essence-back`), en Go.

Cette librairie ne contient **aucune logique métier**. Elle fournit uniquement :

| Package | Contenu |
|---|---|
| `config` | lecture de la configuration (`ES_URL`, `ES_API_KEY`, `ES_USER`, `ES_PASSWORD`, `ES_INDEX_PREFIX`), chargeur de `.env` |
| `es` | client avec retry et health check, types de mapping, gestion d'index/alias/ILM, ingestion bulk idempotente, recherche paginée |
| `pagination` | curseurs opaques base64 pour `search_after` |
| `models` | enveloppe de réponse paginée générique `PaginatedResponse[T]` |
| `estest` | faux serveur Elasticsearch pour les tests unitaires des backends |

Elasticsearch reste manipulé en JSON brut (`map[string]any`) : les requêtes métier
(scripts Painless, agrégations `composite`, `geo_distance`...) sont des documents JSON.
Le client est `go-elasticsearch` **v8** : rester sur la ligne 8.x tant que le cluster l'est
(le client v9 envoie un header `Accept` incompatible avec un serveur 8.x).

## Installation

```bash
go get github.com/openhexa-fr/core@latest
```

Tant qu'aucun tag n'est publié, un backend peut pointer sur un checkout local :

```bash
go mod edit -replace github.com/openhexa-fr/core=../core
```

## Utilisation

```go
settings, _ := config.LoadESSettings()
client, err := es.Connect(ctx, settings) // attend le cluster, backoff exponentiel (~100 s max)

es.CreateIndex(ctx, client, "openhexa-stations-000001", mapping, nil)
es.EnsureAlias(ctx, client, "openhexa-stations", "openhexa-stations-000001")

// Ingestion idempotente : l'`_id` est déterministe, une ré-ingestion écrase.
docs := []es.Document{{"_id": es.MakeDocumentID(stationID), "ville": "Paris"}}
ok, failed, err := es.BulkIndex(ctx, client, "openhexa-stations", slices.Values(docs))

// Recherche paginée par `search_after` (jamais de from/size).
page, err := es.Paginate(ctx, client, es.PaginateParams{
	Index: "openhexa-stations",
	Query: query,
	Sort:  []map[string]any{{"gazole": "asc"}, {"_seq_no": "asc"}},
	Size:  20,
})
response, err := models.FromPage[Station](page)
```

### Compatibilité des `_id` avec la version Python

`es.MakeDocumentID` rend ses parts comme `str()` en Python (`nil` → `None`,
`true` → `True`, `1.0` → `1.0`, ...), pour que les `_id` des documents déjà indexés
restent identiques. Les tests comparent avec des valeurs calculées par l'implémentation
Python.

### Différences de comportement avec la version Python

- Le client n'est plus un singleton global : `es.Connect` retourne un `*elasticsearch.Client`
  à passer explicitement aux fonctions.
- `paginate` : une taille hors `[0, 1000]` retourne `es.ErrInvalidSize` (une `ValueError` en Python).
- `es.GetSource` et `es.DecodeHits` sont de nouveaux helpers.
- `pagination.DecodeCursor` retourne les nombres en `json.Number`, pour conserver les
  entiers (dates en millisecondes) sans perte.

## Développement

```bash
go vet ./...
go test -race ./...
```
