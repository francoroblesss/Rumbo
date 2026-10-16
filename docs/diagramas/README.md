# Diagramas C4

| Diagrama | Fuente | Imagen |
| --- | --- | --- |
| Nivel 1: contexto | [`c4-contexto.puml`](c4-contexto.puml) | [`c4-contexto.svg`](c4-contexto.svg) |
| Nivel 2: contenedores | [`c4-contenedores.puml`](c4-contenedores.puml) | [`c4-contenedores.svg`](c4-contenedores.svg) |

Los diagramas se escriben como código con [C4-PlantUML](https://github.com/plantuml-stdlib/C4-PlantUML), que viene incluido en PlantUML. Así se versionan y se revisan en los pull requests como el resto del repositorio. Se muestran en [`ARCHITECTURE.md`](../ARCHITECTURE.md).

**Regenerar las imágenes** después de editar un `.puml` (desde la raíz del repositorio, con Docker):
```bash
docker run --rm -v "$PWD/docs/diagramas:/data" plantuml/plantuml:1.2025.2 -tsvg -charset UTF-8 /data/c4-contexto.puml /data/c4-contenedores.puml
```
En Git Bash (Windows), anteponer `MSYS_NO_PATHCONV=1` y usar `$(pwd -W)` en lugar de `$PWD`.

Hasta la Entrega 2 los diagramas eran PDF exportados sin fuente editable; siguen en el historial de Git (commit `4557f39`).
