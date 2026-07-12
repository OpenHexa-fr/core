"""Ingestion bulk idempotente vers Elasticsearch."""

from __future__ import annotations

import hashlib
from collections.abc import Iterable
from typing import Any

import structlog
from elasticsearch import AsyncElasticsearch
from elasticsearch.helpers import async_bulk

logger = structlog.get_logger(__name__)

_DOCUMENT_ID_LENGTH = 16


def make_document_id(*parts: object) -> str:
    """Construit un `_id` déterministe à partir de `parts` jointes.

    Garantit l'idempotence de l'ingestion : les mêmes parts produisent toujours
    le même `_id`, donc une ré-ingestion écrase le document existant plutôt que
    d'en créer un doublon. Le hash SHA256 des parts est tronqué à 16 caractères
    hexadécimaux.
    """
    joined = "|".join(str(part) for part in parts)
    return hashlib.sha256(joined.encode("utf-8")).hexdigest()[:_DOCUMENT_ID_LENGTH]


def _to_bulk_action(index_alias: str, document: dict[str, Any]) -> dict[str, Any]:
    source = {key: value for key, value in document.items() if key != "_id"}
    return {
        "_op_type": "index",
        "_index": index_alias,
        "_id": document["_id"],
        "_source": source,
    }


async def bulk_index(
    client: AsyncElasticsearch,
    index_alias: str,
    documents: Iterable[dict[str, Any]],
) -> tuple[int, int]:
    """Indexe `documents` en masse dans `index_alias`.

    L'écriture se fait toujours via l'alias, jamais directement sur un index nommé.
    Chaque document doit porter une clé `_id` (voir `make_document_id`).

    Retourne (nombre de succès, nombre d'erreurs).
    """
    actions = (_to_bulk_action(index_alias, document) for document in documents)
    success, errors = await async_bulk(client, actions, raise_on_error=False)
    error_count = len(errors) if isinstance(errors, list) else int(errors)
    if error_count:
        logger.warning(
            "elasticsearch_bulk_index_errors", index_alias=index_alias, errors=error_count
        )
    return success, error_count
