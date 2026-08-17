"""Modèles Pydantic de base partagés par les backends OpenHexa."""

from __future__ import annotations

from typing import Generic, TypeVar

from pydantic import BaseModel, ConfigDict

T = TypeVar("T", bound=BaseModel)


class BaseDocument(BaseModel):
    """Base commune pour tout document indexé dans Elasticsearch."""

    model_config = ConfigDict(populate_by_name=True, extra="ignore")


class BasePaginatedResponse(BaseModel, Generic[T]):
    """Enveloppe de réponse paginée pour les endpoints de recherche `search_after`."""

    items: list[T]
    total: int
    # "eq" (décompte exact) ou "gte" (plafonné par `track_total_hits`, 10 000
    # par défaut). `None` quand l'appelant ne renseigne pas l'information.
    total_relation: str | None = None
    # Curseur opaque à renvoyer tel quel pour obtenir la page suivante.
    next_cursor: str | None = None
    # Valeurs de tri brutes. Conservé pour les clients existants ; préférer
    # `next_cursor`, qui n'oblige pas le client à retyper les valeurs.
    next_search_after: list[object] | None = None
