from abc import ABC, abstractmethod
from pathlib import Path

import httpx

class BaseDownloader(ABC):
    @property          # (1) se lit "obj.url", SANS parenthèses
    @abstractmethod    # (2) obligatoire à fournir
    def url(self) -> str:
        ...

    @property
    def chunk_size(self) -> int:
        return 8192
    
    async def download(self, dest: Path) -> Path:
        # Client HTTP async (follow_redirects : suit les redirections).
        async with httpx.AsyncClient(follow_redirects=True) as client:
            # Requête GET en streaming : le corps n'est pas chargé en RAM.
            async with client.stream("GET", self.url) as reponse:
                reponse.raise_for_status()  # erreur si réponse 4xx/5xx
                # Fichier destination en écriture binaire ("wb" = octets).
                with open(dest, "wb") as f:
                    # Écrit le flux paquet par paquet : la RAM reste plate.
                    async for chunk in reponse.aiter_bytes(self.chunk_size):
                        f.write(chunk)
        return dest  # chemin du fichier téléchargé
