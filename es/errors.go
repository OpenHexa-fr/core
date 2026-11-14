package es

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/elastic/go-elasticsearch/v8/esapi"
)

// ErrNotFound est retournée quand Elasticsearch répond 404 (document ou index absent).
var ErrNotFound = errors.New("elasticsearch: introuvable")

// APIError décrit une réponse d'erreur d'Elasticsearch.
type APIError struct {
	Status int
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("elasticsearch: réponse %d : %s", e.Status, e.Body)
}

// Is permet `errors.Is(err, ErrNotFound)` sur une APIError 404.
func (e *APIError) Is(target error) bool {
	return target == ErrNotFound && e.Status == 404
}

// checkResponse transforme une réponse en erreur si son statut est >= 300 et
// referme toujours le corps quand il y a erreur. En cas de succès, le corps
// reste ouvert : c'est à l'appelant de le lire puis de le fermer.
func checkResponse(res *esapi.Response) error {
	if !res.IsError() {
		return nil
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	return &APIError{Status: res.StatusCode, Body: strings.TrimSpace(string(body))}
}
