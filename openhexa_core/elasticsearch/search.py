"""Recherche et pagination Elasticsearch."""

from __future__ import annotations

from collections.abc import AsyncIterator
from typing import Any

from elasticsearch import AsyncElasticsearch

_DEFAULT_PAGE_SIZE = 200
_MAX_RESULT_WINDOW = 1000


async def count(client: AsyncElasticsearch, index: str) -> int:
    """Retourne le nombre de documents visibles dans `index` (alias ou nom d'index).

    Utile pour exposer un statut de disponibilité des données (ex. bandeau
    "synchronisation en cours" côté frontend) sans avoir à exécuter une recherche.
    """
    response = await client.count(index=index)
    return int(response["count"])


def build_filters(**kwargs: Any) -> list[dict[str, Any]]:
    """Construit des clauses `filter` Elasticsearch à partir de kwargs.

    - Une valeur `None` ou une collection vide est ignorée.
    - Une liste/tuple/set devient une clause `terms`.
    - Une valeur scalaire devient une clause `term`.
    """
    filters: list[dict[str, Any]] = []
    for field, value in kwargs.items():
        if value is None:
            continue
        if isinstance(value, list | tuple | set):
            if not value:
                continue
            filters.append({"terms": {field: list(value)}})
        else:
            filters.append({"term": {field: value}})
    return filters


async def paginate(
    client: AsyncElasticsearch,
    index: str,
    query: dict[str, Any],
    sort: list[dict[str, Any]],
    search_after: list[Any] | None = None,
    size: int = _DEFAULT_PAGE_SIZE,
    source: list[str] | None = None,
) -> dict[str, Any]:
    """Retourne une page de résultats en pagination `search_after`.

    Ne jamais utiliser `from/size` au-delà de 1 000 résultats : `sort` doit
    comporter un tie-breaker stable (typiquement `_id`) pour que `search_after`
    soit déterministe.

    `source` restreint les champs rapatriés depuis Elasticsearch (`_source`
    filtering) : utile pour les requêtes à gros `size` dont l'appelant n'exploite
    qu'une poignée de champs (ex. alimenter une carte), où le coût dominant est
    la lecture et la sérialisation du `_source` complet. `None` (défaut) rapatrie
    le document entier.
    """
    if size > _MAX_RESULT_WINDOW:
        raise ValueError(f"size must not exceed {_MAX_RESULT_WINDOW}")

    search_kwargs: dict[str, Any] = {"index": index, "query": query, "sort": sort, "size": size}
    if search_after is not None:
        search_kwargs["search_after"] = search_after
    if source is not None:
        search_kwargs["source"] = source

    response = await client.search(**search_kwargs)
    hits = response["hits"]["hits"]
    next_search_after = hits[-1]["sort"] if hits else None

    return {
        "hits": hits,
        "total": response["hits"]["total"]["value"],
        "next_search_after": next_search_after,
    }


async def search_after_all(
    client: AsyncElasticsearch,
    index: str,
    query: dict[str, Any],
    sort_field: str,
    size: int = _DEFAULT_PAGE_SIZE,
) -> AsyncIterator[dict[str, Any]]:
    """Itère sur tous les documents correspondant à `query`, page par page.

    `sort_field` est complété par un tri secondaire sur `_seq_no` pour garantir
    un ordre total stable, requis pour que `search_after` avance sans sauter ni
    répéter de documents. `_id` ne peut pas servir de tie-breaker (sa fielddata
    est désactivée par défaut) et `_shard_doc` nécessite un point-in-time, que
    cette API stateless ne gère pas.
    """
    sort = [{sort_field: "asc"}, {"_seq_no": "asc"}]
    search_after: list[Any] | None = None

    while True:
        page = await paginate(
            client, index=index, query=query, sort=sort, search_after=search_after, size=size
        )
        hits = page["hits"]
        if not hits:
            return
        for hit in hits:
            yield hit
        search_after = page["next_search_after"]
