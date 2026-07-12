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
    next_search_after: list[object] | None = None
