# openhexa-core

Couche partagée d'accès à Elasticsearch pour les backends OpenHexa (`immo-back`, `essence-back`).

Cette librairie ne contient **aucune logique métier**. Elle fournit uniquement :

- la configuration de connexion à Elasticsearch (`config.py`)
- un client async avec retry et health check (`elasticsearch/client.py`)
- des types de mapping réutilisables (`elasticsearch/mappings.py`)
- des helpers de gestion d'index et d'alias (`elasticsearch/index.py`)
- des helpers d'ingestion bulk idempotente (`elasticsearch/ingestion.py`)
- des helpers de recherche paginée (`elasticsearch/search.py`)
- des modèles Pydantic de base (`models/base.py`)

## Installation

```bash
pip install openhexa-core
```

En développement, depuis un backend du monorepo :

```bash
pip install -e ../core
```

## Configuration

Variables d'environnement (voir `ESSettings`) :

```env
ES_URL=http://localhost:9200
ES_USER=elastic
ES_PASSWORD=changeme
ES_INDEX_PREFIX=openhexa
```

## Développement

```bash
pip install -e ".[dev]"
pytest
ruff check .
mypy .
```
