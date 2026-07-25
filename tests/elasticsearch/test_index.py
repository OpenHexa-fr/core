"""Tests de la gestion des index/alias Elasticsearch."""

from __future__ import annotations

from unittest.mock import AsyncMock

from openhexa_core.elasticsearch.index import create_index, ensure_alias


async def test_create_index_creates_when_absent() -> None:
    client = AsyncMock()
    client.indices.exists.return_value = False
    mappings = {"properties": {"id": {"type": "keyword"}}}

    created = await create_index(client, "openhexa-dvf-000001", mappings)

    assert created is True
    client.indices.create.assert_called_once_with(
        index="openhexa-dvf-000001", mappings=mappings, settings={}
    )
    client.indices.put_mapping.assert_not_called()


async def test_create_index_merges_new_fields_when_present() -> None:
    client = AsyncMock()
    client.indices.exists.return_value = True
    mappings = {"properties": {"prix_m2": {"type": "float"}}}

    created = await create_index(client, "openhexa-dvf-000001", mappings)

    assert created is False
    client.indices.create.assert_not_called()
    client.indices.put_mapping.assert_called_once_with(
        index="openhexa-dvf-000001", properties={"prix_m2": {"type": "float"}}
    )


async def test_ensure_alias_creates_when_absent() -> None:
    client = AsyncMock()
    client.indices.exists_alias.return_value = False

    created = await ensure_alias(client, "openhexa-dvf", "openhexa-dvf-000001")

    assert created is True
    client.indices.put_alias.assert_called_once_with(
        index="openhexa-dvf-000001", name="openhexa-dvf"
    )


async def test_ensure_alias_noop_when_present() -> None:
    client = AsyncMock()
    client.indices.exists_alias.return_value = True

    created = await ensure_alias(client, "openhexa-dvf", "openhexa-dvf-000001")

    assert created is False
    client.indices.put_alias.assert_not_called()
