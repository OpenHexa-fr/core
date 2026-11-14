package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadESSettings(t *testing.T) {
	t.Setenv("ES_URL", "http://localhost:9200")
	t.Setenv("ES_USER", "elastic")
	t.Setenv("ES_PASSWORD", "changeme")
	t.Setenv("ES_API_KEY", "")
	t.Setenv("ES_INDEX_PREFIX", "")

	settings, err := LoadESSettings()

	if err != nil {
		t.Fatal(err)
	}
	want := ESSettings{URL: "http://localhost:9200", User: "elastic", Password: "changeme", IndexPrefix: "openhexa"}
	if settings != want {
		t.Errorf("settings = %+v", settings)
	}
}

func TestLoadESSettingsRequiresURL(t *testing.T) {
	t.Setenv("ES_URL", "")
	if _, err := LoadESSettings(); err == nil {
		t.Error("ES_URL manquant : erreur attendue")
	}
}

func TestInt(t *testing.T) {
	t.Setenv("N_OK", " 42 ")
	t.Setenv("N_BAD", "abc")
	t.Setenv("N_EMPTY", "")

	if n, err := Int("N_OK", 1); err != nil || n != 42 {
		t.Errorf("n=%d err=%v", n, err)
	}
	if n, err := Int("N_EMPTY", 7); err != nil || n != 7 {
		t.Errorf("défaut : n=%d err=%v", n, err)
	}
	if _, err := Int("N_BAD", 1); err == nil {
		t.Error("valeur non numérique : erreur attendue")
	}
}

func TestLoadDotEnv(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	content := "# commentaire\n\nDOTENV_A=valeur\nexport DOTENV_B=\"entre guillemets\"\nDOTENV_C=du-fichier\nsans-egal\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOTENV_C", "de-l-environnement") // l'environnement prime sur le fichier
	os.Unsetenv("DOTENV_A")
	os.Unsetenv("DOTENV_B")
	t.Cleanup(func() { os.Unsetenv("DOTENV_A"); os.Unsetenv("DOTENV_B") })

	if err := LoadDotEnv(path); err != nil {
		t.Fatal(err)
	}

	for key, want := range map[string]string{
		"DOTENV_A": "valeur", "DOTENV_B": "entre guillemets", "DOTENV_C": "de-l-environnement",
	} {
		if got := os.Getenv(key); got != want {
			t.Errorf("%s = %q, attendu %q", key, got, want)
		}
	}
}

func TestLoadDotEnvMissingFileIsNotAnError(t *testing.T) {
	if err := LoadDotEnv(filepath.Join(t.TempDir(), "absent.env")); err != nil {
		t.Errorf("fichier absent : %v", err)
	}
}
