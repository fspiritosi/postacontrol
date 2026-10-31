package mail

import (
	"context"
	"errors"
	"slices"
	"time"

	"postacontrol/core/imapx"
	"postacontrol/core/model"
	"postacontrol/core/store"
)

const (
	// indexBatch es cuántos cuerpos baja el indexador de una cuenta por
	// pasada antes de descansar.
	indexBatch = 10
	// indexPause es el descanso entre pasadas mientras quedan pendientes.
	indexPause = 2 * time.Second
	// indexIdle es la espera cuando no hay nada que bajar (o todo lo que
	// queda es de cuentas que no se pueden usar por ahora). También es lo que
	// espera una cuenta para volver a probar después de un problema de
	// conexión o de un rechazo del servidor.
	indexIdle = time.Minute
)

// StartIndexing lanza el indexador: una goroutine que baja en segundo plano
// los cuerpos que faltan en la caché, de lo más nuevo a lo más viejo, para que
// la búsqueda local los encuentre y los snippets de la lista mejoren.
//
//   - Baja lo mismo que abrir un correo (texto, HTML e imágenes en línea,
//     nunca adjuntos) con fetchBody, que abre la carpeta con EXAMINE y lee con
//     BODY.PEEK: nunca marca \Seen.
//   - Cada mensaje es un withConn propio: el lock de la conexión de la cuenta
//     se suelta entre mensaje y mensaje, así que lo que pide el usuario espera
//     a lo sumo un mensaje, no un lote. Cada pasada toma como mucho indexBatch
//     mensajes por cuenta y después descansa indexPause.
//   - Saltea las cuentas en pausa por contraseña rechazada (authPaused), y lo
//     vuelve a mirar antes de cada mensaje.
//   - Un mensaje que falla 3 veces (borrado en el servidor, MIME roto, el
//     servidor lo rechaza) se saltea para siempre; los problemas de conexión
//     no cuentan como fallas.
//
// Al terminar cada lote emite EventFolderUpdated por cada carpeta en la que
// indexó algo. No emite EventSyncError: withConn ya avisa los problemas de
// conexión. Cancelar ctx o Close lo frenan. Llamarla de nuevo no hace nada.
// No bloquea.
func (c *Core) StartIndexing(ctx context.Context) {
	c.lifeMu.Lock()
	if c.indexStarted || c.closed {
		c.lifeMu.Unlock()
		return
	}
	c.indexStarted = true
	c.indexRunning++
	c.lifeMu.Unlock()
	ok := c.spawn(func(life context.Context) {
		defer func() {
			c.lifeMu.Lock()
			c.indexRunning--
			c.lifeMu.Unlock()
		}()
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		defer context.AfterFunc(life, cancel)()
		c.indexLoop(ctx)
	})
	if !ok { // el núcleo se cerró en el medio
		c.lifeMu.Lock()
		c.indexRunning--
		c.lifeMu.Unlock()
	}
}

// indexLoop repite pasadas hasta que ctx se cancele. Descansa indexPause si
// la pasada avanzó e indexIdle si no.
func (c *Core) indexLoop(ctx context.Context) {
	// retryAt anota hasta cuándo no se prueba una cuenta cuyo lote se cortó
	// por la conexión o por un rechazo del servidor: sin esto, con la red
	// caída el indexador volvería a conectar (y withConn a avisar el error)
	// cada indexPause.
	retryAt := map[int64]time.Time{}
	for ctx.Err() == nil {
		wait := c.indexIdle
		if c.indexPass(ctx, retryAt) {
			wait = c.indexPause
		}
		if !sleepCtx(ctx, wait) {
			return
		}
	}
}

// indexPass hace un lote por cuenta. Devuelve true si alguna avanzó.
func (c *Core) indexPass(ctx context.Context, retryAt map[int64]time.Time) bool {
	accs, err := c.store.ListAccounts(ctx)
	if err != nil {
		return false
	}
	progress := false
	for _, a := range accs {
		if ctx.Err() != nil {
			return false
		}
		if c.authPaused(a.ID) || time.Now().Before(retryAt[a.ID]) {
			continue
		}
		delete(retryAt, a.ID)
		advanced, cut := c.indexAccount(ctx, a.ID)
		progress = progress || advanced
		if cut {
			retryAt[a.ID] = time.Now().Add(c.indexIdle)
		}
	}
	return progress
}

// indexAccount baja un lote de cuerpos de la cuenta. advanced dice si cambió
// algo (cuerpos guardados, fallas anotadas o una carpeta resincronizada); cut,
// si el lote se cortó por un problema que no es del mensaje y conviene
// esperar antes de volver a probar la cuenta.
//
// Cómo se trata cada error:
//   - contexto, red, contraseña o TLS: corta el lote sin anotar fallas (la
//     contraseña rechazada además deja la cuenta en pausa, lo hace withConn);
//   - "stale" (la carpeta se recreó en el servidor): corta el lote y
//     resincroniza esa carpeta, que resetea la caché. Sin esto una carpeta
//     que no es la Entrada (la sincronización periódica solo mira la Entrada)
//     trabaría el indexador de la cuenta hasta que el usuario la abra;
//   - NO/BAD del servidor: anota la falla y corta el lote, porque puede ser
//     del servidor y no del mensaje (por ejemplo, un límite de uso): sin el
//     corte, un servidor que rechaza todo agotaría los reintentos de lotes
//     enteros cada pocos segundos;
//   - el resto (el mensaje ya no está, MIME roto, un error de la caché):
//     anota la falla y sigue con el siguiente.
func (c *Core) indexAccount(ctx context.Context, accountID int64) (advanced, cut bool) {
	ms, err := c.store.NextToIndex(ctx, accountID, indexBatch)
	if err != nil {
		return false, false
	}
	var touched []int64
	defer func() {
		if c.ctxErr(ctx) != nil {
			return
		}
		for _, id := range touched {
			c.emit(EventFolderUpdated, map[string]any{"folderId": id})
		}
	}()
	for _, d := range ms {
		if c.ctxErr(ctx) != nil || c.authPaused(accountID) {
			return advanced, false
		}
		err := c.indexOne(ctx, d)
		if c.indexHook != nil {
			c.indexHook(d.ID, err)
		}
		if err == nil {
			advanced = true
			if !slices.Contains(touched, d.FolderID) {
				touched = append(touched, d.FolderID)
			}
			continue
		}
		if c.ctxErr(ctx) != nil || isConnProblem(err) {
			return advanced, true
		}
		if userErrorCode(err) == "stale" {
			// Los errores de la resincronización los avisa withConn; si
			// falla, se vuelve a probar después de indexIdle.
			if c.SyncFolder(ctx, d.FolderID) != nil {
				return advanced, true
			}
			return true, false
		}
		if nerr := c.store.NoteIndexFailure(ctx, d.ID); nerr != nil && !errors.Is(nerr, store.ErrNotFound) {
			return advanced, true // la caché no anda: mejor esperar
		}
		advanced = true
		if userErrorCode(err) == "server" {
			return advanced, true
		}
	}
	return advanced, false
}

// indexOne baja el cuerpo de un mensaje y lo guarda en la caché, que lo suma
// al índice de búsqueda (triggers de messages_fts).
func (c *Core) indexOne(ctx context.Context, d model.MessageDetail) error {
	row, err := c.fetchBody(ctx, d)
	if err != nil {
		return err
	}
	if err := c.store.SetBody(ctx, d.ID, row); err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	return nil
}

// isConnProblem dice si err es de la conexión o del contexto y no del mensaje.
func isConnProblem(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, imapx.ErrNetwork) {
		return true
	}
	switch userErrorCode(err) {
	case "network", "auth", "tls":
		return true
	}
	return false
}

func userErrorCode(err error) string {
	var ue *UserError
	if errors.As(err, &ue) {
		return ue.Code
	}
	return ""
}
