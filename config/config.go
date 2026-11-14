// Package config lit la configuration des backends OpenHexa depuis l'environnement.
//
// Les noms de variables sont ceux de la version Python (pydantic-settings) :
// ES_URL, ES_API_KEY, ES_USER, ES_PASSWORD, ES_INDEX_PREFIX.
package config

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"strings"
)

// ESSettings regroupe les paramètres de connexion à Elasticsearch, communs à
// tous les backends OpenHexa.
type ESSettings struct {
	URL         string
	APIKey      string
	User        string
	Password    string
	IndexPrefix string
}

// DefaultIndexPrefix est le préfixe d'index utilisé quand ES_INDEX_PREFIX est absent.
const DefaultIndexPrefix = "openhexa"

// LoadESSettings lit les paramètres Elasticsearch depuis les variables
// d'environnement. ES_URL est obligatoire.
func LoadESSettings() (ESSettings, error) {
	settings := ESSettings{
		URL:         strings.TrimSpace(os.Getenv("ES_URL")),
		APIKey:      os.Getenv("ES_API_KEY"),
		User:        os.Getenv("ES_USER"),
		Password:    os.Getenv("ES_PASSWORD"),
		IndexPrefix: String("ES_INDEX_PREFIX", DefaultIndexPrefix),
	}
	if settings.URL == "" {
		return ESSettings{}, errors.New("config: ES_URL est obligatoire")
	}
	return settings, nil
}

// String retourne la variable d'environnement `key`, ou `def` si elle est absente ou vide.
func String(key, def string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return def
}

// Int retourne la variable d'environnement `key` convertie en entier, ou `def`
// si elle est absente ou vide. Une valeur non numérique est une erreur : mieux
// vaut échouer au démarrage que d'ignorer silencieusement une faute de frappe.
func Int(key string, def int) (int, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return def, nil
	}
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("config: %s=%q n'est pas un entier", key, raw)
	}
	return value, nil
}

// LoadDotEnv charge un fichier .env (lignes KEY=VALUE, commentaires `#`) dans
// l'environnement du process, sans écraser les variables déjà définies. Un
// fichier absent n'est pas une erreur : en production, l'environnement suffit.
func LoadDotEnv(path string) error {
	file, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		key = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(key), "export "))
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if _, exists := os.LookupEnv(key); exists || key == "" {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return err
		}
	}
	return scanner.Err()
}
