package pagination

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestRoundtripPreservesValueTypes(t *testing.T) {
	// Un tri mélange entiers (dates), flottants (prix, distances) et chaînes.
	sortValues := []any{json.Number("1704067200000"), json.Number("2500.75"), "abc", json.Number("42")}

	cursor, err := EncodeCursor(sortValues)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeCursor(cursor)
	if err != nil {
		t.Fatal(err)
	}

	if len(decoded) != len(sortValues) {
		t.Fatalf("longueur %d, attendu %d", len(decoded), len(sortValues))
	}
	for i := range sortValues {
		if decoded[i] != sortValues[i] {
			t.Errorf("valeur %d : %#v, attendu %#v", i, decoded[i], sortValues[i])
		}
	}
}

func TestRoundtripFromGoNumbers(t *testing.T) {
	cursor, err := EncodeCursor([]any{1704067200000, 2500.75})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeCursor(cursor)
	if err != nil {
		t.Fatal(err)
	}
	if decoded[0] != json.Number("1704067200000") || decoded[1] != json.Number("2500.75") {
		t.Errorf("décodé : %#v", decoded)
	}
}

func TestCursorIsURLSafeAndUnpadded(t *testing.T) {
	cursor, err := EncodeCursor([]any{map[string]any{"a": strings.Repeat("b", 40)}})
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"+", "/", "="} {
		if strings.Contains(cursor, forbidden) {
			t.Errorf("le curseur %q contient %q", cursor, forbidden)
		}
	}
}

func TestMalformedCursorIsRejected(t *testing.T) {
	// Un curseur bricolé est une erreur du client, pas une panne du serveur.
	for _, cursor := range []string{"pas-du-base64!", "", "e30", "W3sn", "W10xMg"} {
		if _, err := DecodeCursor(cursor); !errors.Is(err, ErrInvalidCursor) {
			t.Errorf("DecodeCursor(%q) : erreur %v, attendu ErrInvalidCursor", cursor, err)
		}
	}
}

func TestNonListPayloadIsRejected(t *testing.T) {
	// `search_after` attend une liste : tout autre JSON valide doit être refusé.
	forged := base64.RawURLEncoding.EncodeToString([]byte(`{"nope":1}`))
	if _, err := DecodeCursor(forged); !errors.Is(err, ErrInvalidCursor) {
		t.Errorf("erreur %v, attendu ErrInvalidCursor", err)
	}
	null := base64.RawURLEncoding.EncodeToString([]byte(`null`))
	if _, err := DecodeCursor(null); !errors.Is(err, ErrInvalidCursor) {
		t.Errorf("null : erreur %v, attendu ErrInvalidCursor", err)
	}
}

func TestPaddedCursorIsAccepted(t *testing.T) {
	padded := base64.URLEncoding.EncodeToString([]byte(`["a"]`))
	if _, err := DecodeCursor(padded); err != nil {
		t.Errorf("curseur avec padding refusé : %v", err)
	}
}
