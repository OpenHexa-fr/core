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
        async with httpx.AsyncClient(follow_redirects=True) as client:      # cran 1
            async with client.stream("GET", self.url) as reponse:           # cran 2
                reponse.raise_for_status()
                with open(dest, "wb") as f:                                 # cran 3
                    async for chunk in reponse.aiter_bytes(self.chunk_size):# cran 4
                        f.write(chunk)
        return dest
