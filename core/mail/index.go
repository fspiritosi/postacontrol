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
	// queda es de cuentas que no se pueden usar por ahora).
	indexIdle = time.Minute
)

// indexBackoff son las esperas antes de volver a probar una cuenta cuyo lote
// se cortó (sin red, el servidor rechaza, la carpeta se recreó, la caché
// falla): avanza un escalón por cada corte seguido y se queda en el último.
// Vuelve al primero cuando la cuenta indexa un mensaje.
var indexBackoff = []time.Duration{time.Minute, 2 * time.Minute, 5 * time.Minute, 10 * time.Minute}

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
//     servidor lo rechaza, corta la conexión cada vez que se lo pide) se
//     saltea para siempre; los problemas de conexión no cuentan como fallas.
//   - Una cuenta cuyo lote se cortó espera según indexBackoff antes de
//     volver a probarla.
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
	// Sin la espera por cuenta, con la red caída el indexador volvería a
	// conectar (y withConn a avisar el error) cada indexPause.
	b := newAccountBackoff(c.indexBackoff)
	for ctx.Err() == nil {
		wait := c.indexIdle
		if c.indexPass(ctx, b) {
			wait = c.indexPause
		}
		if !sleepCtx(ctx, wait) {
			return
		}
	}
}

// indexPass hace un lote por cuenta. Devuelve true si alguna avanzó.
func (c *Core) indexPass(ctx context.Context, b *accountBackoff) bool {
	accs, err := c.store.ListAccounts(ctx)
	if err != nil {
		return false
	}
	progress := false
	for _, a := range accs {
		if ctx.Err() != nil {
			return false
		}
		if c.authPaused(a.ID) || !b.ready(a.ID, time.Now()) {
			continue
		}
		r := c.indexAccount(ctx, a.ID)
		progress = progress || r.advanced
		if r.indexed {
			b.reset(a.ID)
		}
		if r.cut {
			b.cut(a.ID, time.Now())
		}
	}
	return progress
}

// accountBackoff lleva, por cuenta, cuántos cortes seguidos tuvo y hasta
// cuándo no se la vuelve a probar. Lo usa una sola goroutine (indexLoop).
type accountBackoff struct {
	sched   []time.Duration
	step    map[int64]int
	retryAt map[int64]time.Time
}

func newAccountBackoff(sched []time.Duration) *accountBackoff {
	return &accountBackoff{sched: sched, step: map[int64]int{}, retryAt: map[int64]time.Time{}}
}

// ready dice si la cuenta ya se puede probar.
func (b *accountBackoff) ready(accountID int64, now time.Time) bool {
	return !now.Before(b.retryAt[accountID])
}

// cut anota un corte: la cuenta espera el escalón actual y avanza uno.
// Devuelve la espera.
func (b *accountBackoff) cut(accountID int64, now time.Time) time.Duration {
	if len(b.sched) == 0 {
		return 0
	}
	step := b.step[accountID]
	d := b.sched[step]
	b.step[accountID] = min(step+1, len(b.sched)-1)
	b.retryAt[accountID] = now.Add(d)
	return d
}

// reset vuelve la cuenta al primer escalón (indexó algo).
func (b *accountBackoff) reset(accountID int64) {
	delete(b.step, accountID)
	delete(b.retryAt, accountID)
}

// batchResult es cómo terminó el lote de una cuenta. advanced: cambió algo
// (cuerpos guardados, fallas anotadas o una carpeta resincronizada). indexed:
// guardó al menos un cuerpo. cut: se cortó por algo que no es del mensaje y
// conviene esperar (indexBackoff) antes de volver a probar la cuenta.
type batchResult struct {
	advanced, indexed, cut bool
}

// indexAccount baja un lote de cuerpos de la cuenta.
//
// Cómo se trata cada error:
//   - contexto, contraseña o TLS: corta el lote sin anotar fallas (la
//     contraseña rechazada además deja la cuenta en pausa, lo hace withConn);
//   - red: puede ser la red o el mensaje (un BODYSTRUCTURE que no se puede
//     leer, un servidor que corta la conexión al pedir ese UID). Se prueba la
//     cuenta con un NOOP: si anda, la falla es del mensaje (se anota y se
//     sigue con el siguiente); si no, corta el lote sin anotar fallas. Sin
//     esto un mensaje así, siendo el más nuevo, trabaría la cuenta para
//     siempre;
//   - "stale" (la carpeta se recreó en el servidor): corta el lote y
//     resincroniza esa carpeta, que resetea la caché. Sin esto una carpeta
//     que no es la Entrada (la sincronización periódica solo mira la Entrada)
//     trabaría el indexador de la cuenta hasta que el usuario la abra;
//   - NO/BAD del servidor: anota la falla y corta el lote, porque puede ser
//     del servidor y no del mensaje (por ejemplo, un límite de uso): sin el
//     corte, un servidor que rechaza todo agotaría los reintentos de lotes
//     enteros cada pocos segundos;
//   - la caché no puede guardar el cuerpo o anotar la falla: corta el lote;
//   - el resto (el mensaje ya no está, MIME roto): anota la falla y sigue con
//     el siguiente.
func (c *Core) indexAccount(ctx context.Context, accountID int64) (r batchResult) {
	ms, err := c.store.NextToIndex(ctx, accountID, indexBatch)
	if err != nil {
		r.cut = c.ctxErr(ctx) == nil
		return r
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
			return r
		}
		storeFailed, err := c.indexOne(ctx, d)
		if c.indexHook != nil {
			c.indexHook(d.ID, err)
		}
		if err == nil {
			r.advanced, r.indexed = true, true
			if !slices.Contains(touched, d.FolderID) {
				touched = append(touched, d.FolderID)
			}
			continue
		}
		if c.ctxErr(ctx) != nil {
			return r
		}
		switch {
		case storeFailed:
			r.cut = true
			return r
		case errors.Is(err, store.ErrNotFound):
			continue // el mensaje se borró de la caché mientras tanto
		case errors.Is(err, imapx.ErrNetwork):
			// ¿La red o este mensaje? Los errores del NOOP los avisa withConn.
			if !c.accountReachable(ctx, accountID) {
				r.cut = c.ctxErr(ctx) == nil
				return r
			}
		case isConnProblem(err):
			r.cut = true
			return r
		case userErrorCode(err) == "stale":
			// Los errores de la resincronización los avisa withConn. Con o
			// sin éxito se espera: un servidor que cambia el UIDVALIDITY a
			// cada rato no tiene que resincronizar cada indexPause.
			if c.SyncFolder(ctx, d.FolderID) == nil {
				r.advanced = true
			}
			r.cut = c.ctxErr(ctx) == nil
			return r
		}
		if nerr := c.store.NoteIndexFailure(ctx, d.ID); nerr != nil && !errors.Is(nerr, store.ErrNotFound) {
			r.cut = c.ctxErr(ctx) == nil // la caché no anda: mejor esperar
			return r
		}
		r.advanced = true
		if userErrorCode(err) == "server" {
			r.cut = true
			return r
		}
	}
	return r
}

// indexOne baja el cuerpo de un mensaje y lo guarda en la caché, que lo suma
// al índice de búsqueda (triggers de messages_fts). storeFailed dice si lo que
// falló fue guardar en la caché (y no el mensaje); si el mensaje ya no está en
// la caché, el error es store.ErrNotFound y storeFailed es false.
func (c *Core) indexOne(ctx context.Context, d model.MessageDetail) (storeFailed bool, err error) {
	row, err := c.fetchBody(ctx, d)
	if err != nil {
		return false, err
	}
	if err := c.store.SetBody(ctx, d.ID, row); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return false, err
		}
		return true, c.ctxOrUserError(ctx, err)
	}
	return false, nil
}

// accountReachable dice si la cuenta se puede usar ahora (conecta si hace
// falta y manda NOOP).
func (c *Core) accountReachable(ctx context.Context, accountID int64) bool {
	err := c.withConn(ctx, accountID, func(_ context.Context, conn *imapx.Conn) error { return conn.Noop() })
	return err == nil
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
