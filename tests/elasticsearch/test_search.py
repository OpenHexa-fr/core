"""Tests des helpers de recherche et pagination."""

from __future__ import annotations

from unittest.mock import AsyncMock

import pytest

from openhexa_core.elasticsearch.search import build_filters, count, paginate, search_after_all


def test_build_filters_ignores_none() -> None:
    filters = build_filters(commune=None, code_postal="13001")

    assert filters == [{"term": {"code_postal": "13001"}}]


def test_build_filters_builds_terms_for_collections() -> None:
    filters = build_filters(type_local=["Maison", "Appartement"])

    assert filters == [{"terms": {"type_local": ["Maison", "Appartement"]}}]


def test_build_filters_ignores_empty_collection() -> None:
    assert build_filters(type_local=[]) == []


async def test_paginate_rejects_size_above_result_window() -> None:
    with pytest.raises(ValueError):
        await paginate(AsyncMock(), "openhexa-dvf", {}, sort=[{"date": "asc"}], size=1001)


async def test_paginate_returns_next_search_after() -> None:
    client = AsyncMock()
    client.search.return_value = {
        "hits": {
            "hits": [{"_id": "1", "sort": ["2024-01-01", "1"]}],
            "total": {"value": 1},
        }
    }

    page = await paginate(client, "openhexa-dvf", {}, sort=[{"date": "asc"}, {"_id": "asc"}])

    assert page["total"] == 1
    assert page["next_search_after"] == ["2024-01-01", "1"]


async def test_count_returns_document_count() -> None:
    client = AsyncMock()
    client.count.return_value = {"count": 42}

    assert await count(client, "openhexa-dvf") == 42
    client.count.assert_called_once_with(index="openhexa-dvf")


async def test_search_after_all_iterates_until_empty_page() -> None:
    client = AsyncMock()
    client.search.side_effect = [
        {
            "hits": {
                "hits": [{"_id": "1", "sort": ["a", "1"]}],
                "total": {"value": 2},
            }
        },
        {
            "hits": {
                "hits": [],
                "total": {"value": 2},
            }
        },
    ]

    results = [
        hit async for hit in search_after_all(client, "openhexa-dvf", {}, sort_field="date")
    ]

    assert len(results) == 1
    assert results[0]["_id"] == "1"
