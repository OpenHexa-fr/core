"""Client Elasticsearch singleton partagé par tous les backends OpenHexa."""

from __future__ import annotations

import asyncio

import structlog
from elasticsearch import AsyncElasticsearch

from openhexa_core.config import ESSettings

logger = structlog.get_logger(__name__)

_client: AsyncElasticsearch | None = None

# Budget total ~100s : un cluster ES conteneurisé peut rester injoignable un
# moment après que son propre healthcheck Docker le déclare "healthy".
_MAX_RETRIES = 15
_BASE_DELAY_SECONDS = 1.0
_MAX_DELAY_SECONDS = 10.0


def _auth_kwargs(settings: ESSettings) -> dict[str, str | tuple[str, str]]:
    """Résout les kwargs d'auth à passer à AsyncElasticsearch.

    Préfère l'API key (accès scopé) si fournie, sinon retombe sur user/password.
    """
    if settings.es_api_key:
        return {"api_key": settings.es_api_key}
    if settings.es_user and settings.es_password:
        return {"basic_auth": (settings.es_user, settings.es_password)}
    raise ValueError("ESSettings doit fournir soit es_api_key, soit es_user et es_password")


async def get_client(settings: ESSettings | None = None) -> AsyncElasticsearch:
    """Retourne le client Elasticsearch singleton, en le créant si nécessaire.

    La connexion est retentée avec un backoff exponentiel si le cluster n'est pas
    encore joignable (utile au démarrage d'un conteneur avant qu'ES ne soit prêt).
    """
    global _client
    if _client is not None:
        return _client

    resolved_settings = settings or ESSettings()
    client = AsyncElasticsearch(resolved_settings.es_url, **_auth_kwargs(resolved_settings))

    attempt = 1
    while True:
        if await health_check(client):
            _client = client
            return _client
        if attempt >= _MAX_RETRIES:
            await client.close()
            raise ConnectionError(
                f"Elasticsearch unreachable at {resolved_settings.es_url} "
                f"after {_MAX_RETRIES} attempts"
            )
        delay = min(_BASE_DELAY_SECONDS * (2 ** (attempt - 1)), _MAX_DELAY_SECONDS)
        logger.warning("elasticsearch_unreachable_retry", attempt=attempt, delay=delay)
        await asyncio.sleep(delay)
        attempt += 1


async def health_check(client: AsyncElasticsearch) -> bool:
    """Vérifie que le cluster Elasticsearch répond."""
    try:
        return await client.ping()
    except Exception:  # noqa: BLE001 - toute erreur réseau vaut "down"
        return False


async def close_client() -> None:
    """Ferme le client singleton et réinitialise l'état (utile pour les tests)."""
    global _client
    if _client is not None:
        await _client.close()
        _client = None
