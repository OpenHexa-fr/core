from pydantic_settings import BaseSettings, SettingsConfigDict

class CoreSettings(BaseSettings):
    es_url: str = "http://localhost:9200"
    model_config = SettingsConfigDict(env_prefix="OPENHEXA_", env_file=".env")
