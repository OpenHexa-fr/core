"""Fixtures partagées pour les tests de openhexa_core."""

from __future__ import annotations

from collections.abc import AsyncIterator

import pytest

from openhexa_core.config import ESSettings
from openhexa_core.elasticsearch import client as client_module


@pytest.fixture
def settings() -> ESSettings:
    return ESSettings(es_url="http://localhost:9200", es_user="elastic", es_password="changeme")


@pytest.fixture(autouse=True)
async def _reset_client_singleton() -> AsyncIterator[None]:
    """Réinitialise le singleton du client ES avant et après chaque test."""
    await client_module.close_client()
    yield
    await client_module.close_client()
