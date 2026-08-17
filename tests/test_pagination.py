"""Tests des curseurs de pagination opaques."""

from __future__ import annotations

import pytest

from openhexa_core.pagination import decode_cursor, encode_cursor


def test_roundtrip_preserves_value_types() -> None:
    """Un tri mélange entiers (dates), flottants (prix, distances) et chaînes."""
    sort_values = [1704067200000, 2500.75, "abc", 42]

    assert decode_cursor(encode_cursor(sort_values)) == sort_values


def test_cursor_is_url_safe_and_unpadded() -> None:
    cursor = encode_cursor([{"a": "b" * 40}])

    assert "+" not in cursor
    assert "/" not in cursor
    assert "=" not in cursor


@pytest.mark.parametrize("cursor", ["pas-du-base64!", "", "e30", "W3sn"])
def test_malformed_cursor_raises_value_error(cursor: str) -> None:
    """Un curseur bricolé est une erreur du client, pas une panne du serveur."""
    with pytest.raises(ValueError):
        decode_cursor(cursor)


def test_cursor_encoding_a_non_list_payload_is_rejected() -> None:
    """`search_after` attend une liste : tout autre JSON valide doit être refusé."""
    import base64
    import json

    forged = base64.urlsafe_b64encode(json.dumps({"nope": 1}).encode()).decode().rstrip("=")

    with pytest.raises(ValueError):
        decode_cursor(forged)
