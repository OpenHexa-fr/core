"""Gestion des index, alias et politiques ILM Elasticsearch."""

from __future__ import annotations

from typing import Any

import structlog
from elasticsearch import AsyncElasticsearch

logger = structlog.get_logger(__name__)


async def create_index(
    client: AsyncElasticsearch,
    name: str,
    mappings: dict[str, Any],
    settings: dict[str, Any] | None = None,
) -> bool:
    """Crée l'index `name` avec le mapping explicite fourni s'il n'existe pas déjà.

    S'il existe déjà, les nouveaux champs du mapping y sont fusionnés (PUT
    mapping additif) : Elasticsearch ne recrée jamais les champs déjà présents,
    donc cette opération est sans risque et permet de faire évoluer le
    mapping explicite d'un domaine sans réindexation complète.

    Retourne True si l'index a été créé, False s'il existait déjà.
    """
    if await client.indices.exists(index=name):
        await client.indices.put_mapping(index=name, **mappings)
        return False

    await client.indices.create(index=name, mappings=mappings, settings=settings or {})
    logger.info("elasticsearch_index_created", index=name)
    return True


async def ensure_alias(client: AsyncElasticsearch, alias: str, index: str) -> bool:
    """Fait pointer `alias` vers `index`, en créant l'alias s'il n'existe pas déjà.

    Toute écriture des backends doit passer par l'alias, jamais par le nom d'index brut.
    """
    if await client.indices.exists_alias(name=alias, index=index):
        return False

    await client.indices.put_alias(index=index, name=alias)
    logger.info("elasticsearch_alias_created", alias=alias, index=index)
    return True


async def setup_ilm(client: AsyncElasticsearch, policy_name: str, policy: dict[str, Any]) -> None:
    """Configure (crée ou met à jour) une politique ILM."""
    await client.ilm.put_lifecycle(name=policy_name, policy=policy)
    logger.info("elasticsearch_ilm_configured", policy_name=policy_name)
