// Package models contient les types de réponse partagés par les backends OpenHexa.
package models

import "github.com/openhexa-fr/core/es"

// PaginatedResponse est l'enveloppe de réponse des endpoints de recherche `search_after`.
type PaginatedResponse[T any] struct {
	Items []T   `json:"items"`
	Total int64 `json:"total"`
	// TotalRelation vaut "eq" (décompte exact) ou "gte" (plafonné par
	// `track_total_hits`, 10 000 par défaut). nil quand l'appelant ne
	// renseigne pas l'information.
	TotalRelation *string `json:"total_relation"`
	// NextCursor est le curseur opaque à renvoyer tel quel pour obtenir la page suivante.
	NextCursor *string `json:"next_cursor"`
	// NextSearchAfter contient les valeurs de tri brutes. Conservé pour les
	// clients existants ; préférer NextCursor, qui n'oblige pas le client à
	// retyper les valeurs.
	NextSearchAfter []any `json:"next_search_after"`
}

// FromPage construit une réponse paginée à partir d'une page de résultats dont
// le `_source` se décode dans T. NextSearchAfter reste nil : l'ancien champ
// n'est renseigné que par les routes qui l'exposent encore.
func FromPage[T any](page es.Page) (PaginatedResponse[T], error) {
	items, err := es.DecodeHits[T](page.Hits)
	if err != nil {
		return PaginatedResponse[T]{}, err
	}
	relation := page.TotalRelation
	return PaginatedResponse[T]{
		Items:         items,
		Total:         page.Total,
		TotalRelation: &relation,
		NextCursor:    page.NextCursor,
	}, nil
}
