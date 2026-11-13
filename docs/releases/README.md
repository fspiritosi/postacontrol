# Releases

Un release se publica solo al subir un tag `vX.Y.Z`: el workflow
`.github/workflows/release.yml` corre `task check`, arma el `.deb` y el `.rpm`
con `task package` y los publica en GitHub junto con `SHA256SUMS`. Los
paquetes se compilan en los servidores de GitHub, no en tu PC.

## Pasos

1. Escribir las notas en `docs/releases/vX.Y.Z.md`, con el mismo formato que
   los anteriores (novedades e instalación), y mergearlas a `main`. Si no hay
   archivo, GitHub arma las notas con la lista de PRs.
2. Crear el tag sobre `main` y subirlo:

   ```sh
   git checkout main && git pull
   git tag -a vX.Y.Z -m "PostaControl X.Y.Z: <resumen>"
   git push origin vX.Y.Z
   ```

3. Seguir el workflow en la pestaña Actions (o con `gh run watch`). Al
   terminar, el release queda publicado como el más reciente.

## Instalar en la PC

```sh
gh release download vX.Y.Z -p '*.rpm' -D /tmp/postacontrol
sudo dnf install /tmp/postacontrol/postacontrol-X.Y.Z-1.x86_64.rpm
```

En Ubuntu o Debian: `-p '*.deb'` y `sudo apt install ./postacontrol_X.Y.Z_amd64.deb`.
Después hay que salir de la app desde la bandeja (Salir) y volver a abrirla.

## Rearmar un release

Si un release ya existe (por ejemplo, un borrador al que le faltan los
paquetes), Actions → release → Run workflow con su tag le sube los paquetes,
conserva sus notas y lo publica.
