"""Tests d'ingestion bulk et de génération d'`_id` déterministe."""

from __future__ import annotations

from unittest.mock import AsyncMock, patch

from openhexa_core.elasticsearch.ingestion import bulk_index, make_document_id


def test_make_document_id_is_deterministic() -> None:
    first = make_document_id("2024-01-01", "13202", "001")
    second = make_document_id("2024-01-01", "13202", "001")

    assert first == second
    assert len(first) == 16


def test_make_document_id_differs_on_different_parts() -> None:
    assert make_document_id("a", "b") != make_document_id("a", "c")


async def test_bulk_index_writes_via_alias() -> None:
    client = AsyncMock()
    documents = [{"_id": "abc123", "nom": "Test"}]

    with patch(
        "openhexa_core.elasticsearch.ingestion.async_bulk",
        new=AsyncMock(return_value=(1, [])),
    ) as mock_bulk:
        success, errors = await bulk_index(client, "openhexa-dvf", documents)

    assert success == 1
    assert errors == 0

    call_args = mock_bulk.call_args
    actions = list(call_args.args[1])
    assert actions[0]["_index"] == "openhexa-dvf"
    assert actions[0]["_id"] == "abc123"
    assert actions[0]["_source"] == {"nom": "Test"}


async def test_bulk_index_reports_error_count() -> None:
    client = AsyncMock()
    documents = [{"_id": "abc123", "nom": "Test"}]

    with patch(
        "openhexa_core.elasticsearch.ingestion.async_bulk",
        new=AsyncMock(return_value=(0, [{"index": {"error": "boom"}}])),
    ):
        success, errors = await bulk_index(client, "openhexa-dvf", documents)

    assert success == 0
    assert errors == 1
