"""Tests du client Elasticsearch singleton."""

from __future__ import annotations

from unittest.mock import AsyncMock, patch

import pytest

from openhexa_core.config import ESSettings
from openhexa_core.elasticsearch.client import _auth_kwargs, get_client, health_check


async def test_health_check_returns_true_when_ping_succeeds() -> None:
    client = AsyncMock()
    client.ping.return_value = True

    assert await health_check(client) is True


async def test_health_check_returns_false_on_exception() -> None:
    client = AsyncMock()
    client.ping.side_effect = ConnectionError("unreachable")

    assert await health_check(client) is False


async def test_get_client_returns_singleton(settings: ESSettings) -> None:
    with patch("openhexa_core.elasticsearch.client.AsyncElasticsearch") as mock_cls:
        instance = AsyncMock()
        instance.ping.return_value = True
        mock_cls.return_value = instance

        first = await get_client(settings)
        second = await get_client(settings)

        assert first is second
        mock_cls.assert_called_once()


def test_auth_kwargs_prefers_api_key_over_basic_auth() -> None:
    settings = ESSettings(
        es_url="http://localhost:9200",
        es_api_key="dGVzdDp0ZXN0",
        es_user="elastic",
        es_password="changeme",
    )

    assert _auth_kwargs(settings) == {"api_key": "dGVzdDp0ZXN0"}


def test_auth_kwargs_falls_back_to_basic_auth() -> None:
    settings = ESSettings(es_url="http://localhost:9200", es_user="elastic", es_password="changeme")

    assert _auth_kwargs(settings) == {"basic_auth": ("elastic", "changeme")}


def test_auth_kwargs_raises_without_credentials() -> None:
    settings = ESSettings(es_url="http://localhost:9200")

    with pytest.raises(ValueError, match="es_api_key"):
        _auth_kwargs(settings)


async def test_get_client_raises_after_max_retries(settings: ESSettings) -> None:
    with (
        patch("openhexa_core.elasticsearch.client.AsyncElasticsearch") as mock_cls,
        patch("openhexa_core.elasticsearch.client.asyncio.sleep", new=AsyncMock()),
    ):
        instance = AsyncMock()
        instance.ping.return_value = False
        mock_cls.return_value = instance

        with pytest.raises(ConnectionError):
            await get_client(settings)
