from openhexa_core.elasticsearch.client import close_client, get_client, health_check
from openhexa_core.elasticsearch.index import create_index, ensure_alias, setup_ilm
from openhexa_core.elasticsearch.ingestion import bulk_index, make_document_id
from openhexa_core.elasticsearch.search import build_filters, count, paginate, search_after_all

__all__ = [
    "get_client",
    "health_check",
    "close_client",
    "create_index",
    "ensure_alias",
    "setup_ilm",
    "bulk_index",
    "make_document_id",
    "search_after_all",
    "paginate",
    "build_filters",
    "count",
]
