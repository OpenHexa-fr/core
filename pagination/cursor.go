// Package pagination fournit des curseurs de pagination opaques.
//
// Les valeurs de tri d'Elasticsearch (`sort`) ne sont pas toutes des chaînes :
// un tri par date renvoie un entier (millisecondes), un tri par prix un
// flottant, un tri `_geo_distance` un flottant lui aussi. Les exposer telles
// quelles en paramètre de requête impose au client de les retyper
// correctement.
//
// Un curseur opaque supprime le problème à la racine : le client le renvoie tel
// qu'il l'a reçu, sans jamais avoir à en interpréter le contenu. C'est aussi ce
// qui permet de faire évoluer les clés de tri sans casser les clients.
package pagination

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
)

// ErrInvalidCursor est retournée pour un curseur illisible — à traduire en 400
// côté API : une valeur tronquée ou bricolée à la main est une erreur du
// client, pas une panne du serveur.
var ErrInvalidCursor = errors.New("curseur illisible")

// EncodeCursor encode les valeurs de tri du dernier hit en curseur
// transmissible en URL (base64 URL-safe, sans padding).
func EncodeCursor(sortValues []any) (string, error) {
	payload, err := json.Marshal(sortValues)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

// DecodeCursor décode un curseur produit par EncodeCursor.
//
// Les nombres sont rendus en json.Number : ils conservent ainsi leur forme
// exacte (un entier reste un entier, pas de perte de précision sur les dates en
// millisecondes) et se re-sérialisent à l'identique vers Elasticsearch.
func DecodeCursor(cursor string) ([]any, error) {
	// Tolère un padding éventuel ajouté par un client.
	payload, err := base64.RawURLEncoding.DecodeString(trimPadding(cursor))
	if err != nil {
		return nil, ErrInvalidCursor
	}

	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	var values []any
	if err := decoder.Decode(&values); err != nil || values == nil {
		return nil, ErrInvalidCursor
	}
	// Refuse tout contenu après la liste.
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, ErrInvalidCursor
	}
	return values, nil
}

func trimPadding(s string) string {
	for len(s) > 0 && s[len(s)-1] == '=' {
		s = s[:len(s)-1]
	}
	return s
}
