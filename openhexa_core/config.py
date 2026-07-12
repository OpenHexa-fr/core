from pydantic_settings import BaseSettings, SettingsConfigDict


class ESSettings(BaseSettings):
    """Paramètres de connexion à Elasticsearch, communs à tous les backends OpenHexa."""

    model_config = SettingsConfigDict(env_file=".env", extra="ignore")

    es_url: str
    es_user: str
    es_password: str
    es_index_prefix: str = "openhexa"
