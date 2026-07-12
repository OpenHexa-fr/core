"""Types de mapping Elasticsearch réutilisables par les backends.

Les mappings des domaines métier (DVF, DPE, stations, ...) doivent être construits
en combinant ces constantes plutôt qu'en laissant Elasticsearch inférer les types.
"""

from typing import Final

KEYWORD: Final[dict[str, str]] = {"type": "keyword"}
TEXT: Final[dict[str, str]] = {"type": "text"}
DATE: Final[dict[str, str]] = {"type": "date"}
GEO_POINT: Final[dict[str, str]] = {"type": "geo_point"}
INTEGER: Final[dict[str, str]] = {"type": "integer"}
FLOAT: Final[dict[str, str]] = {"type": "float"}
BOOLEAN: Final[dict[str, str]] = {"type": "boolean"}
