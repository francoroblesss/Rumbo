# Práctico — Clase 6: Concurrencia y paralelismo en Go

Fuente: repo del docente, `CLASE_6/` (`PRESENTACION/Presentacion.pdf` 17 págs., `DEMO/1..8`, `ACTIVIDADES/`, `ACTIVIDADES_RESUELTAS/`).

"Después de Caché, Solr y RabbitMQ, llegamos al núcleo: cómo un mismo proceso hace varias cosas a la vez de forma elegante y segura." `EXTRA_melisearch` (ejercicio avanzado) está fuera del recorrido y en el `.gitignore` del repo.

## Diccionario
| Término | Significado |
| --- | --- |
| Goroutine | Función liviana que corre de forma independiente |
| Channel | Tubería para comunicar goroutines |
| Select | Espera sobre varios channels a la vez |
| WaitGroup | Espera a que terminen N goroutines |
| Mutex | Candado que protege datos compartidos |
| Race condition | Dos goroutines escribiendo el mismo dato sin control |
| Worker pool | N goroutines fijas procesando trabajos de una cola |
| Context | Cancela o pone timeout a una operación |
| Fan-out / Fan-in | Repartir trabajo entre goroutines y juntar resultados |

## Teoría de la presentación
- **Problema: latencia acumulada.** La ficha de producto consulta tres fuentes independientes — **precio** (base de datos), **stock** (servicio externo), **reviews** (API de terceros); en serie las latencias se suman. Actualizar 10.000 productos uno por uno es inviable.
- **Concurrencia** = estructurar el problema en tareas independientes que avanzan entrelazadas (depende del **diseño**). **Paralelismo** = ejecución física simultánea (depende del **hardware**). Go permite programar concurrente; el runtime decide cuánto paraleliza según los núcleos. (Sin demo: no es determinista.)
- **Goroutines:** `go f(...)`; arrancan con ~**2 KB** de stack; el runtime multiplexa miles sobre pocos threads del SO.
- **Channels:** tuberías tipadas. "**No compartas memoria para comunicarte; comunícate para compartir memoria.**" Sin buffer (`make(chan int)`): el envío bloquea hasta la recepción. Con buffer (`make(chan int, 3)`): envía sin bloquear hasta llenar.
- **Select:** espera sobre varios channels y reacciona al primero listo; con `default` no bloquea. Caso típico: primer resultado entre caché y BD + `time.After(100ms)` como timeout.
- **Race conditions y Mutex:** `mu.Lock(); stock -= 1; mu.Unlock()` (sección crítica). Detectar con `go run -race main.go`.
- **WaitGroup:** `Add(n)` registra pendientes · `Done()` (con `defer`) · `Wait()` bloquea hasta cero.
- **Fan-out / Fan-in:** fase 1 la goroutine principal reparte N búsquedas → fase 2 workers consultan precio/stock/reviews → fase 3 resultados convergen en un channel. Resuelve el catálogo "sin sumar latencias".
- **Worker pool:** una goroutine por tarea puede saturar recursos. "Sin pool: 10.000 productos = 10.000 goroutines simultáneas. Con pool de 50 workers: solo 50 activas."
- **Context:** `context.WithTimeout(context.Background(), 300*time.Millisecond)` + `defer cancel()` (siempre; libera recursos aunque la operación termine antes). Propaga cancelación; si un externo no responde, seguimos sin esperarlo. Una goroutine bloqueada para siempre es un **goroutine leak**.
- **Leaks y deadlocks:** channel sin lector (`go func(){ ch <- 42 }()` sin nadie que lea) = leak garantizado; context con timeout es la red de seguridad.
- **Reglas de oro:** **cierra el channel quien escribe** (nunca quien lee — causa panic) · **usa Context siempre** · **testea con `-race`**.
- Alternativa al Mutex: un channel como semáforo con capacidad 1.

## Demos (`DEMO/`, cada archivo es un programa: `go run main_N.go`; cada paso agrega una idea)
| Demo | Recorrido |
| --- | --- |
| `1_goroutines` | (1) secuencial; (2) `go processUsers()` y `main` termina antes sin esperar; (3) `time.Sleep` en main para dejarla terminar; (4) `go processUser(id)` por usuario: los tres avanzan a la vez. Las goroutines no garantizan orden. |
| `2_channels` | (1) enviar/recibir; (2) sin buffer el emisor espera 5 s a que main reciba ("caja de mano a mano"); (3) igual con `make(chan string, 2)`: el emisor sigue ("pequeña fila"); (4) dos pedidos con funciones de envío y recepción; (5) receptor lento → emisor espera; (6) ídem con buffer 2. |
| `3_select` | (1)/(2) gana el primer channel listo (sin prioridad fija; si ambos están listos, Go elige); (3) `for { select }` hasta la señal de salida y `return`; (4) sin emisor ni `default` → `fatal error: all goroutines are asleep - deadlock!`; (5) con `default` no bloquea. |
| `4_timeout` | `time.After(time.Second)` en el `select`: (1) gana el timeout; (2) gana la respuesta; (3) el límite empezó al inicio, la segunda respuesta llega tarde; (4) límite global de 5 s con respuestas cada 700 ms. "El timeout no cancela mágicamente una función lenta: esta demo deja de esperar"; la cancelación explícita es con `context`. |
| `5_race_conditions_mutex_y_waitgroup` | Stock 10, dos ventas → debe quedar 8. (1) secuencial = 8; (2) goroutines sin mutex: ambas leen 10 y escriben 9 (race); (3) `sync.Mutex` + `defer Unlock` = 8; (4) `sync.WaitGroup` (`Add(2)`, `&sales`, `defer sales.Done()`, `Wait()`). "**Mutex protege el stock. WaitGroup no protege nada: solo espera.**" `go run -race main_1.go / main_2.go`. |
| `6_fan_out_fan_in` | App comparadora de precios (Frávega, Garbarino, Musimundo, Coto, Carrefour con distintos delays): (1) una consulta; (2) fan-out con un channel por comercio (main lee en orden fijo); (3) fan-in: todos al mismo `pricesChannel`, se imprime en orden de llegada; (4)/(5) 3 y 5 comercios. El channel se crea en `main`. |
| `7_worker_pool` | (1) secuencial; (2) una goroutine por producto + WaitGroup (miles de productos = miles de goroutines); (3) **pool de 3 workers** leyendo de `productChannel` sin buffer, `close(productChannel)` al terminar de enviar ("no llegarán más productos"), `for range` termina con channel cerrado y vacío, WaitGroup para esperar workers. (El README dice cinco productos; el código usa 13.) |
| `8_context_timeouts_y_cancelacion` | (1) `context.WithCancel` + `cancelRequest()` manual (la persona cerró la pantalla) → `case <-ctx.Done()`; (2) `context.WithTimeout(300ms)` + `defer cancel()`: "el timeout es solo una forma de cancelar un context"; (3) precio (100 ms), stock (200 ms) y reviews (600 ms) **comparten el mismo context**: los dos primeros llegan, reviews se cancela. |

## Actividades (con variante por formulario: constantes `XXX` a reemplazar)
Módulo `concurrencia-paralelismo/ejercicios-base`, go 1.25. `-race` requiere **cgo + compilador de C** (falla con `-race requires cgo`).

| Ejercicio | Consigna | Solución de referencia (`ACTIVIDADES_RESUELTAS`) |
| --- | --- | --- |
| **Hands On 1** `handson1/race.go` | Reproducir la race: `totalSales` goroutines hacen `stock--` sin sincronizar; `go run -race` y leer el reporte. WaitGroup solo para esperar (no sincroniza el acceso). | Mismo código (100 de stock, 1000 goroutines). El reporte marca **dos accesos en conflicto a la misma dirección, al menos uno de escritura**, con stack trace de ambas goroutines y la línea exacta. Correcto sería -900, rara vez da eso. |
| **Hands On 2** `handson2/race_mutex.go` | Ya viene resuelto con Mutex, para comparar. | `mu.Lock(); stock--; mu.Unlock()` → siempre -900, sin reporte. |
| **Ej. 1** Fan-out/Fan-in | README: ficha con precio, stock y reviews en goroutines + channels (o channel compartido + WaitGroup); medir vs secuencial — el total se acerca a **la fuente más lenta, no a la suma**. Código base: `comparePrices()` con **dos comercios** secuenciales. | `priceChannel := make(chan Price, 2)`, dos goroutines envían, se reciben dos valores. |
| **Ej. 2** Worker pool | Usar `productIDs`, `processDelay`, `workerCount` (variante); comparar tiempos. Objetivo: por qué no lanzar una goroutine por tarea sin control y cómo N workers impacta el throughput. | 3 workers, 5 productos, 300 ms: channel sin buffer, `close`, WaitGroup (mismo patrón que la demo 7). |
| **Ej. 3** Context | Si reviews no responde antes de `timeoutLimit`, devolver `defaultReviews`. Practicar `select` con `ctx.Done()`. | `reviewsChannel` con buffer 1 + `context.WithTimeout(timeoutLimit)` (500 ms) + `select` → `defaultReviews` (0) si gana el timeout. |
| **Ej. 4 (bonus)** Contador seguro | Ver la race en `inseguro`; resolver con (a) **Mutex** y (b) **goroutine dueña del estado** + channel. Resultado correcto: `initialStock - totalSales`. | (a) Mutex solo alrededor de `stock--`; el `time.Sleep` **fuera del lock** (si estuviera adentro, las compras se procesarían de a una). (b) `pedidos := make(chan struct{})`; la goroutine "gestora" tiene `stock` como variable local, hace `for range pedidos { stock-- }` y al cerrarse el channel envía el valor por `resultado`; las 50 compras solo escriben en `pedidos`; tras `wg.Wait()` se hace `close(pedidos)`. Siempre 50, sin reporte de -race. |

- La presentación dice que "los enunciados completos con dominio, datos de entrada y criterios de entrega están en la guía impresa".
- **Inconsistencias del repo:** los READMEs nombran funciones que no existen en el código (`obtenerFichaConcurrente`, `procesarConWorkerPool`, `obtenerFichaConTimeout`, `consultarReviews`; en el código son `comparePrices`, `processWithWorkers`, `getProductReviews`, `getReviews`) y mencionan la carpeta `ejercicios-base/`/`soluciones/`. El README de soluciones describe un Ej. 2 con channels `jobs`/`resultados`, 1000 productos × 50 ms y N=5/N=20, y un Ej. 3 con 300 ms; el código resuelto usa 3 workers/5 productos y 500 ms.
