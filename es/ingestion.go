package es

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"log/slog"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esutil"
)

const documentIDLength = 16

// Document est un document à indexer : un objet JSON portant sa clé `_id`
// (voir MakeDocumentID), qui n'est pas indexée dans le `_source`.
type Document = map[string]any

// MakeDocumentID construit un `_id` déterministe à partir de `parts` jointes
// par `|`.
//
// Garantit l'idempotence de l'ingestion : les mêmes parts produisent toujours
// le même `_id`, donc une ré-ingestion écrase le document existant plutôt que
// d'en créer un doublon. Le hash SHA-256 est tronqué à 16 caractères
// hexadécimaux.
//
// Les parts sont rendues comme `str()` les rendait en Python (nil → "None",
// true → "True", 1.0 → "1.0"), pour que les `_id` des documents déjà indexés
// par la version Python restent identiques : sans cela, une ré-ingestion créerait
// des doublons au lieu d'écraser.
func MakeDocumentID(parts ...any) string {
	rendered := make([]string, len(parts))
	for i, part := range parts {
		rendered[i] = pyStr(part)
	}
	sum := sha256.Sum256([]byte(strings.Join(rendered, "|")))
	return hex.EncodeToString(sum[:])[:documentIDLength]
}

// pyStr reproduit `str(value)` de Python pour les types qui servent de parts d'identifiant.
func pyStr(value any) string {
	switch v := value.(type) {
	case nil:
		return "None"
	case string:
		return v
	case bool:
		if v {
			return "True"
		}
		return "False"
	case json.Number:
		return v.String()
	case float64:
		return pyFloat(v)
	case float32:
		return pyFloat(float64(v))
	}

	switch rv := reflect.ValueOf(value); rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(rv.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(rv.Uint(), 10)
	}
	return fmt.Sprint(value)
}

// pyFloat reproduit `repr(float)` de Python : forme la plus courte qui
// se relit à l'identique, toujours avec une partie décimale ("100.0"), et
// notation exponentielle en dehors de [1e-4, 1e16[.
func pyFloat(f float64) string {
	switch {
	case f != f:
		return "nan"
	case f > 1.7976931348623157e308:
		return "inf"
	case f < -1.7976931348623157e308:
		return "-inf"
	case f == 0:
		if 1/f < 0 {
			return "-0.0"
		}
		return "0.0"
	}

	mantissa, expRaw, _ := strings.Cut(strconv.FormatFloat(f, 'e', -1, 64), "e")
	exp, _ := strconv.Atoi(expRaw)
	sign := ""
	if strings.HasPrefix(mantissa, "-") {
		sign, mantissa = "-", mantissa[1:]
	}
	digits := strings.Replace(mantissa, ".", "", 1)

	switch {
	case exp < -4 || exp >= 16:
		out := digits[:1]
		if len(digits) > 1 {
			out += "." + digits[1:]
		}
		expSign := "+"
		if exp < 0 {
			expSign, exp = "-", -exp
		}
		return fmt.Sprintf("%s%se%s%02d", sign, out, expSign, exp)
	case exp >= 0:
		if len(digits) <= exp+1 {
			return sign + digits + strings.Repeat("0", exp+1-len(digits)) + ".0"
		}
		return sign + digits[:exp+1] + "." + digits[exp+1:]
	default:
		return sign + "0." + strings.Repeat("0", -exp-1) + digits
	}
}

// BulkIndex indexe `docs` en masse dans `indexAlias`.
//
// L'écriture se fait toujours via l'alias, jamais directement sur un index
// nommé. Chaque document doit porter une clé `_id` de type string (voir
// MakeDocumentID). Un document dont l'`_id` existe déjà est remplacé. Les
// documents sont consommés au fil de l'eau : une source volumineuse n'a pas
// besoin d'être chargée en mémoire.
//
// Retourne le nombre de succès et d'échecs unitaires. Une erreur n'est retournée
// que pour un problème qui empêche l'écriture elle-même (document sans `_id`,
// transport, contexte annulé) ; les compteurs restent alors valides.
func BulkIndex(
	ctx context.Context, client *elasticsearch.Client, indexAlias string, docs iter.Seq[Document],
) (success, failed int, err error) {
	result, err := bulk(ctx, client, indexAlias, "index", docs)
	if result.failed > 0 {
		slog.Warn("elasticsearch_bulk_index_errors", "index_alias", indexAlias, "errors", result.failed)
	}
	return result.success, result.failed, err
}

// BulkCreate ajoute `docs` à `target` (data stream ou alias) sans jamais
// écraser un document existant : action `create`, seule acceptée par un data
// stream.
//
// Un document dont l'`_id` existe déjà dans l'index d'écriture est compté
// dans `duplicates` et non comme un échec : renvoyer deux fois le même
// document est sans effet. Dans un data stream, cette garantie ne vaut que pour
// l'index d'écriture courant ; voir ExistingIDs pour filtrer en amont.
func BulkCreate(
	ctx context.Context, client *elasticsearch.Client, target string, docs iter.Seq[Document],
) (created, duplicates, failed int, err error) {
	result, err := bulk(ctx, client, target, "create", docs)
	if result.failed > 0 {
		slog.Warn("elasticsearch_bulk_create_errors", "target", target, "errors", result.failed)
	}
	return result.success, result.conflicts, result.failed, err
}

type bulkResult struct {
	success, conflicts, failed int
}

func bulk(
	ctx context.Context, client *elasticsearch.Client, target, action string, docs iter.Seq[Document],
) (bulkResult, error) {
	var (
		mu       sync.Mutex
		firstErr error
		result   bulkResult
	)
	indexer, err := esutil.NewBulkIndexer(esutil.BulkIndexerConfig{
		Client: client,
		OnError: func(_ context.Context, e error) {
			mu.Lock()
			defer mu.Unlock()
			if firstErr == nil {
				firstErr = e
			}
		},
	})
	if err != nil {
		return result, err
	}

	// Les échecs d'un flush entier (transport) ne passent pas par OnFailure :
	// les totaux viennent des statistiques de l'indexer, OnFailure ne sert
	// qu'à isoler les conflits (409) d'un `create`.
	onFailure := func(_ context.Context, _ esutil.BulkIndexerItem, res esutil.BulkIndexerResponseItem, _ error) {
		if action == "create" && res.Status == 409 {
			mu.Lock()
			defer mu.Unlock()
			result.conflicts++
		}
	}

	var addErr error
	for doc := range docs {
		item, e := toBulkItem(target, action, doc)
		if e == nil {
			item.OnFailure = onFailure
			e = indexer.Add(ctx, item)
		}
		if e != nil {
			addErr = e
			break
		}
	}

	closeErr := indexer.Close(ctx)
	stats := indexer.Stats()
	mu.Lock()
	defer mu.Unlock()
	result.success = int(stats.NumIndexed + stats.NumCreated)
	result.failed = int(stats.NumFailed) - result.conflicts
	return result, joinDistinct(addErr, closeErr, firstErr)
}

func toBulkItem(target, action string, doc Document) (esutil.BulkIndexerItem, error) {
	id, ok := doc["_id"].(string)
	if !ok || id == "" {
		return esutil.BulkIndexerItem{}, errors.New("es: document sans clé `_id` de type string")
	}
	source := maps.Clone(doc)
	delete(source, "_id")
	payload, err := json.Marshal(source)
	if err != nil {
		return esutil.BulkIndexerItem{}, fmt.Errorf("es: document %s non sérialisable : %w", id, err)
	}
	// La cible figure dans chaque action (et non dans l'URL) : une écriture ne
	// peut ainsi jamais viser autre chose que la cible demandée.
	return esutil.BulkIndexerItem{
		Action: action, Index: target, DocumentID: id, Body: bytes.NewReader(payload),
	}, nil
}

// joinDistinct réunit les erreurs non nulles en écartant les doublons : une
// annulation de contexte remonte sinon trois fois, par trois chemins.
func joinDistinct(errs ...error) error {
	var distinct []error
	for _, err := range errs {
		if err == nil {
			continue
		}
		if !slices.ContainsFunc(distinct, func(seen error) bool { return seen == err || seen.Error() == err.Error() }) {
			distinct = append(distinct, err)
		}
	}
	return errors.Join(distinct...)
}
