package es

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"reflect"
	"slices"

	"github.com/elastic/go-elasticsearch/v8"

	"github.com/openhexa-fr/core/pagination"
)

const (
	// DefaultPageSize est la taille de page par défaut.
	DefaultPageSize = 200
	// MaxResultWindow est la taille de page maximale acceptée.
	MaxResultWindow = 1000
)

// ErrInvalidSize est retournée quand la taille de page est négative ou dépasse MaxResultWindow.
var ErrInvalidSize = fmt.Errorf("es: size doit être compris entre 0 et %d", MaxResultWindow)

// Hit est un résultat de recherche. `Source` reste en JSON brut : à l'appelant
// de le décoder dans son propre type (voir DecodeHits).
type Hit struct {
	ID     string          `json:"_id"`
	Source json.RawMessage `json:"_source"`
	// Sort contient les valeurs de tri ; les nombres sont des json.Number.
	Sort []any `json:"sort"`
}

// Page est une page de résultats paginée par `search_after`.
type Page struct {
	Hits  []Hit
	Total int64
	// TotalRelation vaut "eq" (compte exact) ou "gte" (plafonné, 10 000 par
	// défaut côté Elasticsearch). Sans cette information, un total de 10 000 se
	// lit à tort comme un décompte exact.
	TotalRelation   string
	NextSearchAfter []any
	// NextCursor est le curseur opaque (voir pagination) de la page suivante,
	// nil sur une page vide.
	NextCursor *string
}

// Count retourne le nombre de documents visibles dans `index` (alias ou nom d'index).
//
// Utile pour exposer un statut de disponibilité des données (ex. bandeau
// "synchronisation en cours" côté frontend) sans avoir à exécuter une recherche.
func Count(ctx context.Context, client *elasticsearch.Client, index string) (int64, error) {
	res, err := client.Count(client.Count.WithContext(ctx), client.Count.WithIndex(index))
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()
	if err := checkResponse(res); err != nil {
		return 0, err
	}
	var body struct {
		Count int64 `json:"count"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return 0, err
	}
	return body.Count, nil
}

// existingIDsChunk est le nombre d'`_id` recherchés par requête : il ne doit
// pas dépasser `index.max_result_window` (10 000 par défaut).
const existingIDsChunk = 10_000

// ExistingIDs retourne, parmi `ids`, ceux déjà présents dans `index` (alias,
// data stream ou nom d'index), tous index sous-jacents confondus.
//
// Sert à dédoublonner avant un BulkCreate dans un data stream, où l'unicité de
// l'`_id` n'est garantie qu'au sein de l'index d'écriture courant. Un `index`
// absent n'est pas une erreur : aucun `_id` n'y existe encore.
func ExistingIDs(
	ctx context.Context, client *elasticsearch.Client, index string, ids []string,
) (map[string]bool, error) {
	existing := map[string]bool{}
	for chunk := range slices.Chunk(ids, existingIDsChunk) {
		body, err := jsonBody(map[string]any{
			"query":   map[string]any{"ids": map[string]any{"values": chunk}},
			"size":    len(chunk),
			"_source": false,
		})
		if err != nil {
			return nil, err
		}
		res, err := client.Search(
			client.Search.WithContext(ctx), client.Search.WithIndex(index), client.Search.WithBody(body),
			client.Search.WithIgnoreUnavailable(true), client.Search.WithAllowNoIndices(true),
		)
		if err != nil {
			return nil, err
		}
		if err := checkResponse(res); err != nil {
			return nil, err
		}
		var parsed struct {
			Hits struct {
				Hits []struct {
					ID string `json:"_id"`
				} `json:"hits"`
			} `json:"hits"`
		}
		err = json.NewDecoder(res.Body).Decode(&parsed)
		res.Body.Close()
		if err != nil {
			return nil, err
		}
		for _, hit := range parsed.Hits.Hits {
			existing[hit.ID] = true
		}
	}
	return existing, nil
}

// Filter est un critère pour BuildFilters.
type Filter struct {
	Field string
	Value any
}

// BuildFilters construit des clauses `filter` Elasticsearch.
//
//   - Une valeur nil (ou un pointeur nil) est ignorée.
//   - Un slice/array devient une clause `terms` ; vide, il est ignoré.
//   - Une valeur scalaire (ou un pointeur vers un scalaire) devient une clause `term`.
//
// L'ordre des filtres est celui des arguments.
func BuildFilters(filters ...Filter) []map[string]any {
	clauses := make([]map[string]any, 0, len(filters))
	for _, f := range filters {
		value := reflect.ValueOf(f.Value)
		for value.IsValid() && value.Kind() == reflect.Pointer {
			if value.IsNil() {
				value = reflect.Value{}
				break
			}
			value = value.Elem()
		}
		if !value.IsValid() {
			continue
		}
		switch value.Kind() {
		case reflect.Slice, reflect.Array:
			if value.Len() == 0 {
				continue
			}
			clauses = append(clauses, map[string]any{"terms": map[string]any{f.Field: value.Interface()}})
		default:
			clauses = append(clauses, map[string]any{"term": map[string]any{f.Field: value.Interface()}})
		}
	}
	return clauses
}

// PaginateParams décrit une recherche paginée.
type PaginateParams struct {
	Index string
	Query map[string]any
	// Sort doit comporter un tie-breaker stable (par ex. `_seq_no`) pour que
	// `search_after` soit déterministe.
	Sort        []map[string]any
	SearchAfter []any
	// Size vaut DefaultPageSize quand il est nul ; au-delà de MaxResultWindow,
	// Paginate retourne ErrInvalidSize.
	Size int
	// Source restreint les champs rapatriés depuis Elasticsearch (`_source`
	// filtering) : utile pour les requêtes à gros `size` dont l'appelant
	// n'exploite qu'une poignée de champs (ex. alimenter une carte), où le coût
	// dominant est la lecture et la sérialisation du `_source` complet. nil
	// rapatrie le document entier.
	Source []string
}

// Paginate retourne une page de résultats en pagination `search_after`.
//
// Ne jamais utiliser `from/size` au-delà de 1 000 résultats.
func Paginate(ctx context.Context, client *elasticsearch.Client, params PaginateParams) (Page, error) {
	size := params.Size
	if size == 0 {
		size = DefaultPageSize
	}
	if size < 0 || size > MaxResultWindow {
		return Page{}, ErrInvalidSize
	}

	request := map[string]any{"query": params.Query, "sort": params.Sort, "size": size}
	if request["query"] == nil || len(params.Query) == 0 {
		request["query"] = map[string]any{"match_all": map[string]any{}}
	}
	if params.SearchAfter != nil {
		request["search_after"] = params.SearchAfter
	}
	if params.Source != nil {
		request["_source"] = params.Source
	}

	response, err := search(ctx, client, params.Index, request)
	if err != nil {
		return Page{}, err
	}

	page := Page{
		Hits:          response.Hits.Hits,
		Total:         response.Hits.Total.Value,
		TotalRelation: response.Hits.Total.Relation,
	}
	if page.TotalRelation == "" {
		page.TotalRelation = "eq"
	}
	if n := len(page.Hits); n > 0 && len(page.Hits[n-1].Sort) > 0 {
		page.NextSearchAfter = page.Hits[n-1].Sort
		cursor, err := pagination.EncodeCursor(page.NextSearchAfter)
		if err != nil {
			return Page{}, err
		}
		page.NextCursor = &cursor
	}
	return page, nil
}

// SearchAfterAll itère sur tous les documents correspondant à `query`, page par page.
//
// `sortField` est complété par un tri secondaire sur `_seq_no` pour garantir un
// ordre total stable, requis pour que `search_after` avance sans sauter ni
// répéter de documents. `_id` ne peut pas servir de tie-breaker (sa fielddata
// est désactivée par défaut) et `_shard_doc` nécessite un point-in-time, que
// cette API stateless ne gère pas.
//
// Une erreur est émise comme dernier élément de la séquence.
func SearchAfterAll(
	ctx context.Context, client *elasticsearch.Client, index string,
	query map[string]any, sortField string, size int,
) iter.Seq2[Hit, error] {
	return func(yield func(Hit, error) bool) {
		sort := []map[string]any{{sortField: "asc"}, {"_seq_no": "asc"}}
		var searchAfter []any
		for {
			page, err := Paginate(ctx, client, PaginateParams{
				Index: index, Query: query, Sort: sort, SearchAfter: searchAfter, Size: size,
			})
			if err != nil {
				yield(Hit{}, err)
				return
			}
			if len(page.Hits) == 0 {
				return
			}
			for _, hit := range page.Hits {
				if !yield(hit, nil) {
					return
				}
			}
			searchAfter = page.NextSearchAfter
		}
	}
}

// GetSource retourne le `_source` du document `id`, ou ErrNotFound s'il n'existe pas.
func GetSource(ctx context.Context, client *elasticsearch.Client, index, id string) (json.RawMessage, error) {
	res, err := client.Get(index, id, client.Get.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if err := checkResponse(res); err != nil {
		return nil, err
	}
	var body struct {
		Source json.RawMessage `json:"_source"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return nil, err
	}
	return body.Source, nil
}

// DecodeHits décode le `_source` de chaque hit dans le type T.
func DecodeHits[T any](hits []Hit) ([]T, error) {
	items := make([]T, 0, len(hits))
	for _, hit := range hits {
		var item T
		if err := json.Unmarshal(hit.Source, &item); err != nil {
			return nil, fmt.Errorf("es: document %s illisible : %w", hit.ID, err)
		}
		items = append(items, item)
	}
	return items, nil
}

type searchResponse struct {
	Hits struct {
		Total struct {
			Value    int64  `json:"value"`
			Relation string `json:"relation"`
		} `json:"total"`
		Hits []Hit `json:"hits"`
	} `json:"hits"`
}

func search(ctx context.Context, client *elasticsearch.Client, index string, request map[string]any) (*searchResponse, error) {
	body, err := jsonBody(request)
	if err != nil {
		return nil, err
	}
	res, err := client.Search(
		client.Search.WithContext(ctx), client.Search.WithIndex(index), client.Search.WithBody(body))
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if err := checkResponse(res); err != nil {
		return nil, err
	}

	decoder := json.NewDecoder(res.Body)
	decoder.UseNumber() // garde les valeurs de tri exactes (dates en ms, flottants)
	var response searchResponse
	if err := decoder.Decode(&response); err != nil {
		return nil, fmt.Errorf("es: réponse de recherche illisible : %w", err)
	}
	return &response, nil
}
