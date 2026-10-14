# ValidadorAlarmas

Herramienta CLI de **solo lectura** que diagnostica en qué etapa del proceso de carga (Loader)
se detuvo una alarma, a partir de su AID (`eventPoId`). No reprocesa, corrige ni reinyecta alarmas.

## Compilación
Requiere Go 1.22+ (solo en compilación; el binario no tiene dependencias externas).

    go mod tidy        # genera go.sum la primera vez
    make build         # deja bin/ValidadorAlarmas (CGO_ENABLED=0, estático, linux/amd64)

## Ejecución
    ./bin/ValidadorAlarmas -config=etc/validador.json -LoaderConfig=etc/loader.example.json

El programa pide el AID por pantalla (letras, números, `-` y `_`, máx. 64), ejecuta V1..V7,
muestra dos barras (general y de la validación en curso; texto plano si stdout no es TTY),
imprime el reporte y lo guarda en `reportsPath`. Códigos de salida: 0 ok, 1 error, 2 uso, 130 interrumpido.
Con SIGINT/SIGTERM cierra conexiones y escribe un reporte parcial.

### Ejemplo de ejecucion
$ ./bin/ValidadorAlarmas -config=etc/validador.json -LoaderConfig=etc/loader.example.json
Ingrese el AID (eventPoId) de la alarma: 123456789
[general 7/7] V7 finalizada: OMITIDA        # (en una terminal se ven dos barras de progreso)

================ REPORTE ValidadorAlarmas ================
AID: 123456789        Ejecución: 2026-10-13 14:32:10 (America/Santiago, -03 UTC-03:00)

[V1] Conexiones ................ OK          (3 gestores, dbprocess, dbfinal)
      - Advertencia: no se verifica la host key SFTP (configure 'knownHosts' en validador.json)
[V2] Ejecución del loader ...... NOK         ENM_CNT/alarma: falló en etapa 'filtrado'
      Último ciclo por gestor y tipo
        Gestor    Tipo    Estado  Etapa fallida  Fecha del ciclo
        --------  ------  ------  -------------  -------------------
        ENM_CNT   alarma  NOK     filtrado       2026-10-13 14:30:02
        ENM_CNT   cese    OK      -              2026-10-13 14:30:02
        ...
[V3] Archivos fuente ........... OK          file_20261013_1400.tar.gz | ENM_CNT | alarma
[V4] Volcado ................... OK          1 registro(s) en PCA_ENM_DATA, coinciden con V3
[V5] Filtros ................... NOK         3G (UMTS): no aplica (objectOfReference no contiene 'SubNetwork=RNC') [objectOfReference='...']; además specificProblem no existe en FMA_TRANSLATE (TIPO='3G')
                                             4G (LTE): excluida (excluida por objectOfReference LIKE '%Router%') [objectOfReference='...Router=1...']
                                             5G (NR): no aplica (objectOfReference no contiene 'NRCellDU' ni 'SubNetwork=NR_REgion')
[V6] Traspaso .................. OMITIDA     V5 no validó la alarma (sin tecnología asignada)
[V7] Panel ..................... OMITIDA     V5 no validó la alarma (sin tecnología asignada)

CONCLUSIÓN: La alarma llegó y fue volcada, pero fue descartada en el filtrado (V5).
  * V2: ENM_CNT/alarma: falló en etapa 'filtrado'
==========================================================

Reporte: ./reports/123456789_20261013_143210.md
Log:     log/ValidadorAlarmas_123456789_20261013_143210.log

## Pruebas
    make test && make vet

## Configuración (`-config`)
| Campo | Descripción |
|---|---|
| `envFile` | Si se informa, reemplaza el `ENVFILE` del LoaderConfig. |
| `logPath`, `logLevel` | Log `ValidadorAlarmas_[AID]_[FECHA_HORA].log` (`trace`, `debug`, `info`, `warn`, `error`). |
| `reportsPath`, `report.format`, `report.namePattern` | `md` (defecto) o `txt`; patrón con `{AID}` y `{FECHA_HORA}` (YYYYMMDD_HHMMSS). |
| `maxArchivosPorGestorTipo` | Defecto 5. |
| `timeouts` | `connectSeconds` y `querySeconds`. |
| `knownHosts` | Ruta a un `known_hosts`. **Vacío = no se verifica la host key SFTP** (V1 lo advierte). |
| `esquema` | Nombres de tablas/columnas supuestos (ver abajo). `{TEC}` se reemplaza por 3G, 4G o 5G. |

## Seguridad
- Credenciales solo en memoria; nunca en pantalla, log ni reporte (se enmascaran contraseñas y se limpian usuarios de los errores).
- Consultas parametrizadas (`?`) dentro de transacciones `READ ONLY`; no hay INSERT/UPDATE/DELETE ni DDL.
- Los identificadores SQL configurables (`esquema`) se validan con regex al cargar.
- El campo `estados` del LoaderConfig se carga pero **se ignora en esta versión**.

## Suposiciones a confirmar
1. `PCA_CICLOS` tiene la columna `fecha` (`esquema.colFechaCiclo`) y `gestor` coincide con el `id` del gestor.
2. La columna del AID en `PCA_ALARMAS_xG`, `PCA_PANEL_xG` y `PCA_HISTORICO_xG` es `eventPoId` (`esquema.colAid*`).
3. Las columnas `MOTIVO` y `fin` existen en `PCA_HISTORICO_xG` (`esquema.colMotivo`, `esquema.colFin`).
4. `PCA_PANEL_xG` se resuelve sin prefijo de esquema en `dbfinal`; `PCA_ALARMAS_xG` y `PCA_HISTORICO_xG` con `o_m.` (`esquema.tabla*`).
5. `PCA_ENM_DATA` contiene `alarmingObject` (usada en exclusiones 4G/5G). Si falta, V5 informa `NOK` con el error de BD y lo señala como pendiente.
6. `ARCHIVO_FUENTE` se compara por nombre base, con o sin `.tar.gz`.
7. Dentro de los `.tar.gz` el AID aparece como texto (CSV/JSON/etc.) y como token completo (no pegado a letras, dígitos o `_`). Otras codificaciones no se detectan.
8. Los archivos a revisar en V3 están en `readyPath`, como indica el LoaderConfig.
9. V6 y V7 evalúan todas las tecnologías que V5 dé por válidas; V5 usa los `GESTOR` hallados en `PCA_ENM_DATA` (V4).
10. El usuario de BD tiene permiso `SELECT` y el servidor admite `START TRANSACTION READ ONLY`.

## Límites
- V3 solo revisa los últimos `maxArchivosPorGestorTipo` archivos por gestor y tipo. Si el AID no aparece, puede estar en archivos más antiguos: aumente el parámetro.
- Cada consulta devuelve como máximo 1000 filas.
