"""Curseurs de pagination opaques.

Les valeurs de tri d'Elasticsearch (`sort`) ne sont pas toutes des chaînes : un
tri par date renvoie un entier (millisecondes), un tri par prix un flottant, un
tri `_geo_distance` un flottant lui aussi. Les exposer telles quelles en
paramètre de requête impose au client de les retyper correctement — et un
`list[str]` côté API les aplatit en chaînes, que seule la coercition
d'Elasticsearch rattrape, tant qu'elle y arrive.

Un curseur opaque supprime le problème à la racine : le client le renvoie tel
qu'il l'a reçu, sans jamais avoir à en interpréter le contenu. C'est aussi ce
qui permet de faire évoluer les clés de tri sans casser les clients.
"""

from __future__ import annotations

import base64
import binascii
import json
from typing import Any


def encode_cursor(sort_values: list[Any]) -> str:
    """Encode les valeurs de tri du dernier hit en curseur transmissible en URL."""
    payload = json.dumps(sort_values, separators=(",", ":")).encode("utf-8")
    return base64.urlsafe_b64encode(payload).decode("ascii").rstrip("=")


def decode_cursor(cursor: str) -> list[Any]:
    """Décode un curseur produit par `encode_cursor`.

    Lève `ValueError` sur un curseur illisible — à traduire en 400 côté API :
    une valeur tronquée ou bricolée à la main est une erreur du client, pas une
    panne du serveur.
    """
    padded = cursor + "=" * (-len(cursor) % 4)
    try:
        payload = base64.urlsafe_b64decode(padded.encode("ascii"))
        values = json.loads(payload)
    except (binascii.Error, UnicodeDecodeError, json.JSONDecodeError) as error:
        raise ValueError("curseur illisible") from error

    if not isinstance(values, list):
        raise ValueError("curseur illisible")
    return values
