// Parser de Server-Sent Events (formato text/event-stream).
//
// El stream de avisos se lee con fetch y no con EventSource, porque EventSource no permite mandar
// el header Authorization (teórico 9, ADR-027) y el token no va en la URL. Por eso hace falta
// separar los eventos a mano: el cuerpo llega en fragmentos arbitrarios (un evento puede quedar
// partido entre dos lecturas, o una lectura traer varios eventos juntos).
//
// Reglas del formato que se aplican (WHATWG HTML, "Server-sent events"):
// - cada línea termina en "\n" (se tolera "\r\n");
// - una línea vacía cierra el evento;
// - una línea que empieza con ":" es un comentario (por ejemplo, el ": ping" de Notificaciones);
// - "campo: valor" (se quita un solo espacio después de los dos puntos); campos event, data e id;
// - varias líneas data se unen con "\n"; un evento sin data no se emite.

export interface EventoSSE {
  /** Nombre del evento (`event:`); "message" si no vino. */
  evento: string
  /** Último `id:` recibido en el stream (se manda como Last-Event-ID al reconectar). */
  id: string | undefined
  /** Contenido de las líneas `data:`, unidas con "\n". */
  datos: string
}

export class LectorSSE {
  private pendiente = ''
  private evento = ''
  private datos: string[] = []
  private ultimoId: string | undefined
  private readonly alEvento: (evento: EventoSSE) => void

  constructor(alEvento: (evento: EventoSSE) => void) {
    this.alEvento = alEvento
  }

  /** Agrega un fragmento de texto del stream y emite los eventos que queden completos. */
  agregar(fragmento: string): void {
    this.pendiente += fragmento
    let fin = this.pendiente.indexOf('\n')
    while (fin >= 0) {
      let linea = this.pendiente.slice(0, fin)
      this.pendiente = this.pendiente.slice(fin + 1)
      if (linea.endsWith('\r')) linea = linea.slice(0, -1)
      this.procesarLinea(linea)
      fin = this.pendiente.indexOf('\n')
    }
  }

  private procesarLinea(linea: string): void {
    if (linea === '') {
      this.emitir()
      return
    }
    if (linea.startsWith(':')) return // comentario

    const dosPuntos = linea.indexOf(':')
    const campo = dosPuntos < 0 ? linea : linea.slice(0, dosPuntos)
    let valor = dosPuntos < 0 ? '' : linea.slice(dosPuntos + 1)
    if (valor.startsWith(' ')) valor = valor.slice(1)

    switch (campo) {
      case 'event':
        this.evento = valor
        break
      case 'data':
        this.datos.push(valor)
        break
      case 'id':
        if (!valor.includes('\0')) this.ultimoId = valor
        break
      // "retry" y los campos desconocidos se ignoran: la reconexión la maneja la web (backoff).
    }
  }

  private emitir(): void {
    if (this.datos.length > 0) {
      this.alEvento({ evento: this.evento || 'message', id: this.ultimoId, datos: this.datos.join('\n') })
    }
    this.evento = ''
    this.datos = []
  }
}
