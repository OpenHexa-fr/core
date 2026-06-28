import asyncio
from pathlib import Path
from openhexa.core.downloader import BaseDownloader

# une sous-classe concrète : on remplit le trou 'url' avec une vraie adresse
class TestDownloader(BaseDownloader):
    @property
    def url(self) -> str:
        return "https://www.gnu.org/licenses/gpl-3.0.txt"   # petit fichier texte fiable

async def main():
    dest = await TestDownloader().download(Path("/tmp/gpl.txt"))
    taille = dest.stat().st_size
    print(f"✅ téléchargé : {dest}  ({taille} octets)")

asyncio.run(main())
