package mail

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"postacontrol/core/htmlx"
	"postacontrol/core/model"
	"postacontrol/core/store"
	"postacontrol/core/taskapp"
)

// Reuniones de Taskapp en el calendario. El calendario las muestra desde una
// caché, sin esperar a Taskapp; cada lectura dispara en el fondo un refresco
// de su rango (y cada tanto hay una vuelta de fondo) que la actualiza. Una reunión creada desde acá con invitados de afuera
// tiene además un "espejo": un evento local (Kind "taskapp") con esos
// invitados, que son los que reciben el .ics. El espejo guarda la última
// versión de la reunión que vieron los externos (la foto); si en Taskapp
// cambia, se marca y se les avisa recién cuando el usuario confirma.

const (
	// msgMeetingUncertain: el POST salió pero no llegó respuesta. Mismo
	// contrato con la UI que msgTaskappOrphan ("no la vuelvas a crear").
	msgMeetingUncertain = "No sabemos si la reunión se creó en Taskapp: revisala ahí antes de volver a intentar; no la vuelvas a crear sin revisar."
	// msgMeetingOrphan: la reunión existe en Taskapp pero el espejo no se guardó.
	msgMeetingOrphan        = "La reunión se creó en Taskapp, pero no se pudo guardar la lista de invitados externos en esta PC; no la vuelvas a crear."
	msgNotMeeting           = "Este evento no es una reunión de Taskapp."
	msgExternalsRemoved     = "La reunión sigue en Taskapp; solo se avisó a los invitados externos."
	msgExternalsNotNotified = "No pudimos avisarles a los invitados externos: "
	// msgMeetingSendFailed: la reunión y el espejo quedaron, pero la
	// invitación no se encoló. Mismo contrato con la UI que msgMeetingOrphan.
	msgMeetingSendFailed = "La reunión se creó en Taskapp, pero no pudimos mandar la invitación a los invitados externos: %s; no la vuelvas a crear."

	// meetingLookback: la API filtra solo por inicio, así que se pide desde
	// una semana antes para traer las que empezaron antes y siguen en curso.
	meetingLookback = 7 * 24 * time.Hour
	// La vuelta de fondo mira desde un día atrás hasta dos semanas adelante.
	meetingRefreshBack  = 24 * time.Hour
	meetingRefreshAhead = 14 * 24 * time.Hour

	// meetingRefreshGap: como mucho un refresco por rango cada tanto.
	meetingRefreshGap = 30 * time.Second
	// meetingStaleAfter: una caché más vieja que esto se muestra como vieja.
	meetingStaleAfter = 15 * time.Minute
	// meetingFetchWait acota cada pedido de reuniones (uno por workspace).
	meetingFetchWait = 10 * time.Second
	// meetingRangeKeep: el estado de un rango sin refrescos hace este tiempo se olvida.
	meetingRangeKeep = time.Hour
)

// meetRange es el estado de los refrescos de un rango [from, to).
type meetRange struct {
	running bool
	started time.Time // cuándo se lanzó el último (reloj real)
	lastErr string    // motivo del último fallo; "" si salió bien
	lastOK  time.Time // c.now() del último refresco bueno
}

// Cambios de una reunión con espejo que todavía no se les avisaron a los externos.
const (
	changeNone      = ""
	changeChanged   = "changed"
	changeCancelled = "cancelled"
	changeDeleted   = "deleted"
)

// TaskappWorkspaces devuelve los workspaces del usuario en Taskapp.
func (c *Core) TaskappWorkspaces(ctx context.Context) ([]model.TaskappWorkspace, error) {
	cl, err := c.taskappClient(ctx)
	if err != nil {
		return nil, err
	}
	ws, err := cl.Workspaces(ctx)
	if err != nil {
		return nil, c.taskappErrCtx(ctx, err)
	}
	out := make([]model.TaskappWorkspace, 0, len(ws))
	for _, w := range ws {
		out = append(out, model.TaskappWorkspace{ID: w.ID, Name: w.Name})
	}
	return out, nil
}

// TaskappMembers devuelve los miembros del workspace.
func (c *Core) TaskappMembers(ctx context.Context, workspaceID int64) ([]model.TaskappMember, error) {
	cl, err := c.taskappClient(ctx)
	if err != nil {
		return nil, err
	}
	ms, err := cl.Members(ctx, workspaceID)
	if err != nil {
		return nil, c.taskappErrCtx(ctx, err)
	}
	out := make([]model.TaskappMember, 0, len(ms))
	for _, m := range ms {
		out = append(out, model.TaskappMember{ID: m.ID, Name: m.Name, Email: m.Email})
	}
	return out, nil
}

// meetingSnap es la foto de una reunión: lo que, si cambia, hay que avisarles
// a los externos.
type meetingSnap struct {
	Title       string `json:"title"`
	Start       string `json:"start"`
	End         string `json:"end"`
	AllDay      bool   `json:"allDay"`
	Location    string `json:"location"`
	Description string `json:"description"`
	Status      string `json:"status"`
}

// meetingSnapshot arma la foto de m. Completar la reunión no les cambia nada
// a los externos: "completed" se guarda como "scheduled".
func meetingSnapshot(m taskapp.Meeting) string {
	status := m.Status
	if status == "completed" {
		status = "scheduled"
	}
	b, _ := json.Marshal(meetingSnap{Title: m.Title, Start: m.Start.UTC().Format(time.RFC3339),
		End: m.End.UTC().Format(time.RFC3339), AllDay: m.AllDay, Location: m.Location,
		Description: m.Description, Status: status})
	return string(b)
}

// isMine dice si el usuario me está en la reunión: la creó o está invitado.
func isMine(m taskapp.Meeting, me int64) bool {
	if m.CreatedBy == me {
		return true
	}
	for _, a := range m.Attendees {
		if a.UserID == me {
			return true
		}
	}
	return false
}

func isTaskappDisconnected(err error) bool {
	var ue *UserError
	return errors.As(err, &ue) && ue.Code == "taskapp-disconnected"
}

func isTaskappAuth(err error) bool {
	var ue *UserError
	return errors.As(err, &ue) && ue.Code == "taskapp-auth"
}

// fetchMeetings trae de Taskapp las reuniones de todos los workspaces que
// empiezan desde una semana antes de from hasta to, y guarda en la caché las
// del usuario. Devuelve todas (no solo las del usuario) y los proyectos.
func (c *Core) fetchMeetings(ctx context.Context, cl *taskapp.Client, from, to time.Time) ([]taskapp.Meeting, map[int64]taskapp.Project, error) {
	me, err := c.taskappMeCached(ctx, cl)
	if err != nil {
		return nil, nil, err
	}
	ws, err := cl.Workspaces(ctx)
	if err != nil {
		return nil, nil, err
	}
	ps, err := c.activeProjects(ctx, cl)
	if err != nil {
		return nil, nil, err
	}
	wide := from.Add(-meetingLookback)
	byWS := make([][]taskapp.Meeting, len(ws))
	errs := make([]error, len(ws))
	var wg sync.WaitGroup
	for i, w := range ws {
		wg.Add(1)
		go func() {
			defer wg.Done()
			wctx, cancel := context.WithTimeout(ctx, meetingFetchWait)
			defer cancel()
			byWS[i], errs[i] = cl.Meetings(wctx, w.ID, wide, to)
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, nil, err
		}
	}
	// Recién con todo leído se toca la caché: una lectura a medias no borra nada.
	now := c.now()
	all := []taskapp.Meeting{}
	for i, w := range ws {
		stored := []store.StoredMeeting{}
		for _, m := range byWS[i] {
			all = append(all, m)
			if !isMine(m, me.ID) {
				continue
			}
			raw, err := json.Marshal(m)
			if err != nil {
				return nil, nil, err
			}
			stored = append(stored, store.StoredMeeting{ID: m.ID, WorkspaceID: w.ID, Start: m.Start, End: m.End, JSON: string(raw)})
		}
		if err := c.store.ReplaceMeetings(ctx, w.ID, wide, to, stored, now); err != nil && c.ctxErr(ctx) == nil {
			slog.Warn("no se pudo guardar la caché de reuniones de Taskapp", "err", err)
		}
	}
	return mineOnly(all, me.ID), projectsByID(ps), nil
}

func mineOnly(ms []taskapp.Meeting, me int64) []taskapp.Meeting {
	out := []taskapp.Meeting{}
	for _, m := range ms {
		if isMine(m, me) {
			out = append(out, m)
		}
	}
	return out
}

func projectsByID(ps []taskapp.Project) map[int64]taskapp.Project {
	out := make(map[int64]taskapp.Project, len(ps))
	for _, p := range ps {
		out[p.ID] = p
	}
	return out
}

// knownProjects son los proyectos en memoria, aunque estén viejos (para
// pintar las reuniones de la caché sin ir a la red).
func (c *Core) knownProjects() map[int64]taskapp.Project {
	c.taskappMu.Lock()
	defer c.taskappMu.Unlock()
	return projectsByID(c.taskappProjs)
}

// meetingsFor devuelve, desde la caché y sin esperar a Taskapp, las
// reuniones del usuario que se superponen con [from, to), más los espejos del
// rango cuya reunión no está, y lanza en el fondo un refresco del rango (ver
// requestMeetingRefresh). stale si el último refresco del rango falló o la
// caché tiene más de meetingStaleAfter; errMsg es el motivo del último fallo.
// Sin Taskapp conectado, nada; con el token rechazado, nada (ni la caché ni
// los espejos) y el motivo, sin refrescar hasta reconectar.
func (c *Core) meetingsFor(ctx context.Context, from, to time.Time) (items []model.CalendarItem, stale bool, at time.Time, errMsg string) {
	if _, err := c.taskappClient(ctx); err != nil {
		if isTaskappDisconnected(err) {
			return nil, false, time.Time{}, ""
		}
		return nil, false, time.Time{}, err.Error()
	}
	cached, oldest, err := c.store.MeetingsInRange(ctx, from, to)
	if err != nil && c.ctxErr(ctx) == nil {
		slog.Warn("no se pudo leer la caché de reuniones de Taskapp", "err", err)
	}
	meetings := make([]taskapp.Meeting, 0, len(cached))
	for _, sm := range cached {
		var m taskapp.Meeting
		if json.Unmarshal([]byte(sm.JSON), &m) == nil {
			meetings = append(meetings, m)
		}
	}
	st := c.requestMeetingRefresh(from, to)
	at = oldest
	if at.IsZero() {
		at = st.lastOK
	}
	stale = st.lastErr != "" || (!at.IsZero() && c.now().Sub(at) > meetingStaleAfter)
	return c.meetingRangeItems(ctx, meetings, c.knownProjects(), from, to), stale, at, st.lastErr
}

func meetingRangeKey(from, to time.Time) string {
	return strconv.FormatInt(from.Unix(), 10) + "-" + strconv.FormatInt(to.Unix(), 10)
}

// requestMeetingRefresh lanza en el fondo el refresco de [from, to), salvo
// que ya haya uno andando para ese rango o el último haya salido hace menos
// de meetingRefreshGap. Devuelve el estado del rango antes de lanzarlo. Al
// terminar avisa a la UI si cambió la caché, alguna marca de los espejos o si
// el rango pasó a fallar o se recuperó.
func (c *Core) requestMeetingRefresh(from, to time.Time) meetRange {
	key := meetingRangeKey(from, to)
	c.meetMu.Lock()
	defer c.meetMu.Unlock()
	if c.meetRanges == nil {
		c.meetRanges = map[string]*meetRange{}
	}
	for k, r := range c.meetRanges {
		if k != key && !r.running && time.Since(r.started) > meetingRangeKeep {
			delete(c.meetRanges, k)
		}
	}
	r := c.meetRanges[key]
	if r == nil {
		r = &meetRange{}
		c.meetRanges[key] = r
	}
	st := *r
	if r.running || (!r.started.IsZero() && time.Since(r.started) < meetingRefreshGap) {
		return st
	}
	r.running, r.started = true, time.Now()
	c.meetWG.Add(1)
	ok := c.spawn(func(life context.Context) {
		defer c.meetWG.Done()
		msg, changed := c.refreshMeetingRange(life, from, to)
		c.meetMu.Lock()
		r.running = false
		failedBefore := r.lastErr != ""
		if life.Err() == nil {
			r.lastErr = msg
			if msg == "" {
				r.lastOK = c.now()
			}
		}
		flipped := failedBefore != (r.lastErr != "")
		c.meetMu.Unlock()
		if changed || (flipped && life.Err() == nil) {
			c.emit(EventCalendarChanged, nil)
		}
	})
	if !ok {
		r.running = false
		c.meetWG.Done()
	}
	return st
}

// resetMeetingRefresh olvida los fallos y la espera entre refrescos de todos
// los rangos (al conectar o desconectar Taskapp): la próxima lectura refresca.
func (c *Core) resetMeetingRefresh() {
	c.meetMu.Lock()
	defer c.meetMu.Unlock()
	for _, r := range c.meetRanges {
		r.started, r.lastErr = time.Time{}, ""
	}
}

// refreshMeetingRange trae de Taskapp las reuniones de [from, to), actualiza
// la caché y marca los cambios de los espejos. Devuelve el motivo si falló y
// si cambió algo de lo que se muestra (la caché del rango, los proyectos o
// las marcas). Sin Taskapp conectado no hace nada.
func (c *Core) refreshMeetingRange(ctx context.Context, from, to time.Time) (errMsg string, changed bool) {
	cl, err := c.taskappClient(ctx)
	if isTaskappDisconnected(err) {
		return "", false
	}
	if err != nil {
		return err.Error(), false
	}
	before, projsBefore := c.meetingCacheSig(ctx, from, to), projectsSig(c.knownProjects())
	mine, _, err := c.fetchMeetings(ctx, cl, from.UTC(), to.UTC())
	if err != nil {
		if c.ctxErr(ctx) != nil {
			return "", false
		}
		return c.taskappErr(err).Error(), false
	}
	marked := c.detectTaskappChanges(ctx, cl, mine, from, to)
	return "", marked || c.meetingCacheSig(ctx, from, to) != before || projectsSig(c.knownProjects()) != projsBefore
}

// meetingCacheSig resume las reuniones de la caché en [from, to) (sin la
// fecha de lectura), para saber si un refresco cambió algo.
func (c *Core) meetingCacheSig(ctx context.Context, from, to time.Time) string {
	ms, _, err := c.store.MeetingsInRange(ctx, from, to)
	if err != nil {
		return "?"
	}
	var b strings.Builder
	for _, m := range ms {
		b.WriteString(strconv.FormatInt(m.ID, 10))
		b.WriteByte(':')
		b.WriteString(m.JSON)
		b.WriteByte('\n')
	}
	return b.String()
}

// projectsSig resume los proyectos (id, nombre y color) que pintan las reuniones.
func projectsSig(ps map[int64]taskapp.Project) string {
	ids := make([]int64, 0, len(ps))
	for id := range ps {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	var b strings.Builder
	for _, id := range ids {
		fmt.Fprintf(&b, "%d|%s|%s\n", id, ps[id].Name, ps[id].Color)
	}
	return b.String()
}

// meetingRangeItems arma los ítems de las reuniones que se superponen con
// [from, to), con su espejo si tienen, y los de los espejos del rango cuya
// reunión no está entre ellas.
func (c *Core) meetingRangeItems(ctx context.Context, meetings []taskapp.Meeting, projs map[int64]taskapp.Project, from, to time.Time) []model.CalendarItem {
	_, web, _ := c.taskappURLs(ctx)
	if web == "" {
		web = defaultTaskappWeb
	}
	items := []model.CalendarItem{}
	shown := map[int64]bool{}
	for _, m := range meetings {
		if !overlaps(m.Start, m.End, from, to) {
			continue
		}
		it := meetingItem(m, projs, web)
		if mirror, ok := c.mirrorFor(ctx, m.ID); ok {
			withMirror(&it, mirror)
		}
		shown[m.ID] = true
		items = append(items, it)
	}
	return append(items, c.orphanMirrorItems(ctx, from, to, shown, web)...)
}

// meetingItem arma el ítem de la reunión, con el color y el nombre de su proyecto.
func meetingItem(m taskapp.Meeting, projs map[int64]taskapp.Project, web string) model.CalendarItem {
	it := model.CalendarItem{Key: "m:" + strconv.FormatInt(m.ID, 10), Source: "taskapp", MeetingID: m.ID,
		Title: m.Title, Location: m.Location, Start: m.Start, End: m.End, AllDay: m.AllDay,
		ProjectName: m.ProjectName, Cancelled: m.Status == "cancelled", URL: meetingURL(web, m.ID),
		People: make([]model.Attendee, 0, len(m.Attendees))}
	for _, a := range m.Attendees {
		it.People = append(it.People, model.Attendee{Email: a.Email, Name: a.Name, Partstat: rsvpPartstat(a.Rsvp)})
	}
	if p, ok := projs[m.ProjectID]; ok && m.ProjectID != 0 {
		it.Color, it.ProjectName = p.Color, p.Name
	}
	return it
}

// rsvpPartstat traduce la respuesta de un asistente de Taskapp a PARTSTAT.
func rsvpPartstat(rsvp string) string {
	switch rsvp {
	case "accepted":
		return "ACCEPTED"
	case "declined":
		return "DECLINED"
	case "maybe":
		return "TENTATIVE"
	}
	return "NEEDS-ACTION"
}

func meetingURL(web string, id int64) string {
	return web + "/agenda?meeting=" + strconv.FormatInt(id, 10)
}

// withMirror le suma al ítem de la reunión lo de su espejo: los invitados
// externos (también a People, después de los de Taskapp) y el cambio
// pendiente de avisarles.
func withMirror(it *model.CalendarItem, mirror model.CalendarEvent) {
	confirmed := 0
	for _, a := range mirror.Attendees {
		if a.Partstat == "ACCEPTED" {
			confirmed++
		}
	}
	it.EventID, it.Attendees, it.Confirmed, it.TaskappChange = mirror.ID, len(mirror.Attendees), confirmed, mirror.TaskappChange
	if it.People == nil {
		it.People = []model.Attendee{}
	}
	it.People = append(it.People, mirror.Attendees...)
}

// mirrorFor trae el espejo de la reunión, si tiene.
func (c *Core) mirrorFor(ctx context.Context, meetingID int64) (model.CalendarEvent, bool) {
	e, err := c.store.EventByMeeting(ctx, meetingID)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) && c.ctxErr(ctx) == nil {
			slog.Warn("no se pudo leer el espejo de la reunión", "meetingId", meetingID, "err", err)
		}
		return model.CalendarEvent{}, false
	}
	return e, true
}

// orphanMirrorItems arma, con los datos del espejo, el ítem de las reuniones
// con espejo en [from, to) que no vinieron (normalmente, borradas en Taskapp).
func (c *Core) orphanMirrorItems(ctx context.Context, from, to time.Time, shown map[int64]bool, web string) []model.CalendarItem {
	evs, err := c.store.EventsInRange(ctx, from, to)
	if err != nil {
		if c.ctxErr(ctx) == nil {
			slog.Warn("no se pudieron leer los espejos de reuniones", "err", err)
		}
		return nil
	}
	var out []model.CalendarItem
	for _, e := range evs {
		if e.Kind != "taskapp" || shown[e.TaskappMeetingID] || !overlaps(e.Start, e.End, from, to) {
			continue
		}
		it := model.CalendarItem{Key: "m:" + strconv.FormatInt(e.TaskappMeetingID, 10), Source: "taskapp",
			MeetingID: e.TaskappMeetingID, Title: e.Title, Location: e.Location, Start: e.Start, End: e.End,
			AllDay: e.AllDay, Cancelled: e.Status == "cancelled", URL: meetingURL(web, e.TaskappMeetingID)}
		withMirror(&it, e)
		out = append(out, it)
	}
	return out
}

// mirrorsToCheck son los espejos de evs que empiezan en [from, to) y no están
// cancelados (a esos externos ya les llegó la cancelación).
func mirrorsToCheck(evs []model.CalendarEvent, from, to time.Time) []model.CalendarEvent {
	var out []model.CalendarEvent
	for _, e := range evs {
		if e.Kind == "taskapp" && e.Status != "cancelled" && !e.Start.Before(from) && e.Start.Before(to) {
			out = append(out, e)
		}
	}
	return out
}

// mirrorChange es la marca que corresponde a un espejo con foto snap según la
// reunión m (nil = Taskapp confirmó que no existe). Si coincide con la foto
// no hay nada que avisar, y se borra la marca que hubiera.
func mirrorChange(m *taskapp.Meeting, snap string) string {
	switch {
	case m == nil:
		return changeDeleted
	case m.Status == "cancelled":
		return changeCancelled
	case meetingSnapshot(*m) != snap:
		return changeChanged
	}
	return changeNone
}

// detectTaskappChanges compara, después de una lectura buena, los espejos que
// empiezan en [from, to) con lo que vino de Taskapp (mine: las del usuario de
// esa lectura) y marca los que cambiaron. Si la reunión de un espejo no vino
// (pudo haberse movido fuera del rango), se pregunta por ella a Taskapp, sin
// calMu: solo un 404 la da por borrada; si se encuentra, se guarda en la
// caché (para avisar con sus datos nuevos); con otro error, el espejo queda
// como estaba. Devuelve si cambió alguna marca.
func (c *Core) detectTaskappChanges(ctx context.Context, cl *taskapp.Client, mine []taskapp.Meeting, from, to time.Time) bool {
	byID := make(map[int64]taskapp.Meeting, len(mine))
	for _, m := range mine {
		byID[m.ID] = m
	}
	evs, err := c.store.EventsInRange(ctx, from, to)
	if err != nil {
		if c.ctxErr(ctx) == nil {
			slog.Warn("no se pudieron revisar los cambios de las reuniones", "err", err)
		}
		return false
	}
	gone := map[int64]bool{}
confirm:
	for _, e := range mirrorsToCheck(evs, from, to) {
		id := e.TaskappMeetingID
		if _, ok := byID[id]; ok {
			continue
		}
		m, err := cl.Meeting(ctx, id)
		switch {
		case err == nil:
			byID[id] = m
			c.cacheMeeting(ctx, m)
		case errors.Is(err, taskapp.ErrNotFound):
			gone[id] = true
		case errors.Is(err, taskapp.ErrUnauthorized):
			_ = c.taskappErr(err) // traba y avisa; las demás tampoco se pueden confirmar
			break confirm
		default:
			if c.ctxErr(ctx) != nil {
				return false
			}
			slog.Warn("no se pudo confirmar la reunión en Taskapp", "meetingId", id, "err", err)
		}
	}

	c.calMu.Lock()
	defer c.calMu.Unlock()
	if evs, err = c.store.EventsInRange(ctx, from, to); err != nil {
		if c.ctxErr(ctx) == nil {
			slog.Warn("no se pudieron revisar los cambios de las reuniones", "err", err)
		}
		return false
	}
	marked := false
	for _, e := range mirrorsToCheck(evs, from, to) {
		var m *taskapp.Meeting
		if found, ok := byID[e.TaskappMeetingID]; ok {
			m = &found
		} else if !gone[e.TaskappMeetingID] {
			continue // no se sabe
		}
		snap, err := c.store.TaskappSnapshot(ctx, e.ID)
		if err != nil {
			slog.Warn("no se pudo leer la foto de la reunión", "eventId", e.ID, "err", err)
			continue
		}
		change := mirrorChange(m, snap)
		if change == e.TaskappChange {
			continue
		}
		if err := c.store.SetTaskappState(ctx, e.ID, snap, change); err != nil {
			slog.Warn("no se pudo marcar el cambio de la reunión", "eventId", e.ID, "err", err)
			continue
		}
		marked = true
	}
	return marked
}

// cacheMeeting guarda una reunión suelta en la caché.
func (c *Core) cacheMeeting(ctx context.Context, m taskapp.Meeting) {
	raw, err := json.Marshal(m)
	if err == nil {
		err = c.store.SaveMeeting(ctx, store.StoredMeeting{ID: m.ID, WorkspaceID: m.WorkspaceID, Start: m.Start, End: m.End,
			JSON: string(raw)}, c.now())
	}
	if err != nil && c.ctxErr(ctx) == nil {
		slog.Warn("no se pudo guardar la reunión en la caché", "meetingId", m.ID, "err", err)
	}
}

// CreateTaskappMeeting crea la reunión en Taskapp con los asistentes de
// Taskapp. Si hay invitados de afuera, guarda el espejo y les manda la
// invitación desde req.AccountID. Una vez que sale el POST no se corta si el
// usuario cancela (como CreateTaskappItem): la reunión nunca se crea dos veces.
func (c *Core) CreateTaskappMeeting(ctx context.Context, req model.TaskappMeetingCreate) (model.CalendarItem, error) {
	e := model.CalendarEvent{Kind: "taskapp", Title: req.Title, Description: req.Description, Location: req.Location,
		AllDay: req.AllDay, Start: req.Start, End: req.End, Status: "confirmed", AccountID: req.AccountID}
	for _, a := range req.External {
		e.Attendees = append(e.Attendees, model.Attendee{Email: a.Email, Name: a.Name})
	}
	if err := validateEvent(&e); err != nil {
		return model.CalendarItem{}, err
	}
	var acc model.Account
	if len(e.Attendees) > 0 {
		var err error
		if acc, err = c.inviteAccount(ctx, req.AccountID); err != nil {
			return model.CalendarItem{}, err
		}
		e.Attendees = withoutEmail(e.Attendees, acc.Email)
		e.Organizer = model.Address{Name: acc.DisplayName, Email: acc.Email}
	}
	cl, err := c.taskappClient(ctx)
	if err != nil {
		return model.CalendarItem{}, err
	}
	_, web, err := c.taskappURLs(ctx)
	if err != nil {
		return model.CalendarItem{}, err
	}

	// Desde el POST hasta guardar el espejo y encolar las invitaciones,
	// Close espera y cancelar no corta.
	done, ok := c.hold()
	if !ok {
		return model.CalendarItem{}, context.Canceled
	}
	defer done()
	if cerr := c.ctxErr(ctx); cerr != nil {
		return model.CalendarItem{}, cerr
	}
	bg := func(d time.Duration) (context.Context, context.CancelFunc) {
		return context.WithTimeout(context.WithoutCancel(ctx), d)
	}
	postCtx, cancelPost := bg(taskappCreateWait)
	created, err := cl.CreateMeeting(postCtx, taskapp.NewMeeting{WorkspaceID: req.WorkspaceID, ProjectID: req.ProjectID,
		Title: e.Title, Description: e.Description, Location: e.Location, Start: e.Start, End: e.End, AllDay: e.AllDay,
		AttendeeIDs: req.AttendeeIDs})
	cancelPost()
	if err != nil {
		var api *taskapp.APIError
		if errors.Is(err, taskapp.ErrUnauthorized) || errors.As(err, &api) {
			return model.CalendarItem{}, c.taskappErr(err)
		}
		// Sin respuesta de Taskapp el pedido pudo haber llegado.
		slog.Warn("taskapp: no se sabe si la reunión se creó", "err", err)
		return model.CalendarItem{}, &UserError{Code: "taskapp-uncertain", Msg: msgMeetingUncertain, Err: err}
	}
	// El aviso sale una vez, al final (con el espejo ya guardado si hay).
	defer c.emit(EventCalendarChanged, nil)
	item := meetingItem(created, c.knownProjects(), web)
	if len(e.Attendees) == 0 {
		return item, nil
	}

	e.UID = fmt.Sprintf("taskapp-%d@postacontrol", created.ID)
	e.TaskappMeetingID = created.ID
	saveCtx, cancelSave := bg(taskappSaveWait)
	defer cancelSave()
	c.calMu.Lock()
	defer c.calMu.Unlock()
	mirror, err := c.saveMirror(saveCtx, e, meetingSnapshot(created))
	if err != nil {
		slog.Warn("taskapp: la reunión se creó pero no se guardó el espejo", "meetingId", created.ID, "err", err)
		return model.CalendarItem{}, &UserError{Code: "taskapp-orphan", Msg: msgMeetingOrphan, Err: err}
	}
	withMirror(&item, mirror)
	if err := c.queueInvitation(saveCtx, acc, mirror, "REQUEST", attendeeAddrs(mirror.Attendees), false); err != nil {
		reason := strings.TrimRight(c.ctxOrUserError(saveCtx, err).Error(), ". ")
		return item, &UserError{Code: "send", Msg: fmt.Sprintf(msgMeetingSendFailed, reason), Err: err}
	}
	return item, nil
}

// meetingText pasa a texto la descripción de una reunión leída de Taskapp
// (que la guarda como HTML) para el espejo, el .ics y el correo a los
// externos. Un texto sin marcas queda como está (conserva sus saltos de
// línea). La que se escribe en el formulario no pasa por acá.
func meetingText(desc string) string {
	if !strings.ContainsAny(desc, "<&") {
		return desc
	}
	return strings.TrimSpace(htmlx.ToText(desc))
}

// saveMirror guarda el espejo nuevo con su foto; si la foto no se guarda,
// borra el espejo (sin foto, la próxima lectura lo daría por cambiado). Se
// llama con calMu tomado.
func (c *Core) saveMirror(ctx context.Context, e model.CalendarEvent, snap string) (model.CalendarEvent, error) {
	if c.meetingMirrorHook != nil {
		if err := c.meetingMirrorHook(); err != nil {
			return model.CalendarEvent{}, err
		}
	}
	saved, err := c.store.SaveEvent(ctx, e, seriesEnd(e))
	if err != nil {
		return model.CalendarEvent{}, err
	}
	if err := c.store.SetTaskappState(ctx, saved.ID, snap, changeNone); err != nil {
		_ = c.store.DeleteEvent(ctx, saved.ID)
		return model.CalendarEvent{}, err
	}
	return saved, nil
}

// mirrorEvent trae el espejo de una reunión; cualquier otro evento es un error.
func (c *Core) mirrorEvent(ctx context.Context, eventID int64) (model.CalendarEvent, error) {
	e, err := c.store.GetEvent(ctx, eventID)
	if err != nil {
		return model.CalendarEvent{}, c.ctxOrUserError(ctx, err)
	}
	if e.Kind != "taskapp" {
		return model.CalendarEvent{}, &UserError{Code: "invalid", Msg: msgNotMeeting}
	}
	return e, nil
}

// cachedMeeting trae la reunión de la caché (la de la última lectura buena).
func (c *Core) cachedMeeting(ctx context.Context, id int64) (taskapp.Meeting, error) {
	sm, err := c.store.CachedMeeting(ctx, id)
	if err != nil {
		return taskapp.Meeting{}, err
	}
	var m taskapp.Meeting
	if err := json.Unmarshal([]byte(sm.JSON), &m); err != nil {
		return taskapp.Meeting{}, err
	}
	return m, nil
}

// NotifyTaskappChange les avisa a los externos el cambio pendiente de la
// reunión: con "changed", la invitación actualizada con los datos de la
// última lectura; cancelada o borrada, la cancelación (y el espejo queda
// cancelado). Sin nada pendiente no hace nada. Si el aviso no sale, el cambio
// queda pendiente para volver a intentar.
func (c *Core) NotifyTaskappChange(ctx context.Context, eventID int64) error {
	c.calMu.Lock()
	defer c.calMu.Unlock()
	e, err := c.mirrorEvent(ctx, eventID)
	if err != nil || e.TaskappChange == changeNone {
		return err
	}
	snap, err := c.store.TaskappSnapshot(ctx, e.ID)
	if err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	m, merr := c.cachedMeeting(ctx, e.TaskappMeetingID)
	if merr != nil && (e.TaskappChange == changeChanged || !errors.Is(merr, store.ErrNotFound)) {
		return c.ctxOrUserError(ctx, merr)
	}
	next := e
	next.Sequence++
	method, update := "CANCEL", false
	if e.TaskappChange == changeChanged {
		next.Title, next.Description, next.Location = m.Title, meetingText(m.Description), m.Location
		next.Start, next.End, next.AllDay = m.Start, m.End, m.AllDay
		method, update = "REQUEST", true
	} else {
		next.Status = "cancelled"
	}
	if merr == nil {
		snap = meetingSnapshot(m)
	}
	var acc model.Account
	if len(next.Attendees) > 0 {
		if acc, err = c.inviteAccount(ctx, next.AccountID); err != nil {
			return err
		}
	}
	saved, err := c.saveEvent(ctx, next)
	if err != nil {
		return err
	}
	defer c.emit(EventCalendarChanged, nil)
	if len(saved.Attendees) > 0 {
		if err := c.queueInvitation(ctx, acc, saved, method, attendeeAddrs(saved.Attendees), update); err != nil {
			return &UserError{Code: "send", Msg: msgExternalsNotNotified + c.ctxOrUserError(ctx, err).Error(), Err: err}
		}
	}
	if err := c.store.SetTaskappState(ctx, saved.ID, snap, changeNone); err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	return nil
}

// DismissTaskappChange descarta el cambio pendiente sin avisarles a los
// externos: la foto pasa a ser lo último que se leyó, así no se vuelve a
// marcar hasta que cambie otra cosa. Cancelada o borrada, el espejo queda
// cancelado.
func (c *Core) DismissTaskappChange(ctx context.Context, eventID int64) error {
	c.calMu.Lock()
	defer c.calMu.Unlock()
	e, err := c.mirrorEvent(ctx, eventID)
	if err != nil || e.TaskappChange == changeNone {
		return err
	}
	snap, err := c.store.TaskappSnapshot(ctx, e.ID)
	if err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	m, err := c.cachedMeeting(ctx, e.TaskappMeetingID)
	switch {
	case err == nil:
		snap = meetingSnapshot(m)
	case e.TaskappChange == changeChanged || !errors.Is(err, store.ErrNotFound):
		return c.ctxOrUserError(ctx, err)
	}
	if e.TaskappChange != changeChanged && e.Status != "cancelled" {
		e.Status = "cancelled"
		if e, err = c.saveEvent(ctx, e); err != nil {
			return err
		}
	}
	if err := c.store.SetTaskappState(ctx, e.ID, snap, changeNone); err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	c.emit(EventCalendarChanged, nil)
	return nil
}

// RemoveTaskappExternals les manda la cancelación a los invitados externos y
// borra el espejo. La reunión sigue en Taskapp.
func (c *Core) RemoveTaskappExternals(ctx context.Context, eventID int64) (model.RespondResult, error) {
	c.calMu.Lock()
	defer c.calMu.Unlock()
	e, err := c.mirrorEvent(ctx, eventID)
	if err != nil {
		return model.RespondResult{}, err
	}
	if len(e.Attendees) > 0 {
		acc, err := c.inviteAccount(ctx, e.AccountID)
		if err != nil {
			return model.RespondResult{}, err
		}
		gone := e
		gone.Sequence++
		gone.Status = "cancelled"
		if err := c.queueInvitation(ctx, acc, gone, "CANCEL", attendeeAddrs(gone.Attendees), false); err != nil {
			return model.RespondResult{}, &UserError{Code: "send", Msg: msgExternalsNotNotified + c.ctxOrUserError(ctx, err).Error(), Err: err}
		}
	}
	if err := c.store.DeleteEvent(ctx, e.ID); err != nil {
		return model.RespondResult{}, c.ctxOrUserError(ctx, err)
	}
	c.emit(EventCalendarChanged, nil)
	return model.RespondResult{Warning: msgExternalsRemoved}, nil
}

// StartMeetingRefresh trae las reuniones enseguida y después cada every, en
// la ventana [now−1d, now+14d], para notar los cambios aunque el calendario
// no esté abierto. Sin Taskapp conectado (o con el token rechazado) esa
// vuelta no hace nada. No bloquea.
func (c *Core) StartMeetingRefresh(ctx context.Context, every time.Duration) {
	if every <= 0 {
		return
	}
	c.spawn(func(life context.Context) {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		defer context.AfterFunc(life, cancel)()
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			c.refreshMeetings(ctx)
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	})
}

func (c *Core) refreshMeetings(ctx context.Context) {
	if _, err := c.taskappClient(ctx); err != nil {
		return
	}
	now := c.now()
	msg, changed := c.refreshMeetingRange(ctx, now.Add(-meetingRefreshBack), now.Add(meetingRefreshAhead))
	if changed {
		c.emit(EventCalendarChanged, nil)
	}
	if msg != "" && ctx.Err() == nil {
		slog.Warn("no se pudieron traer las reuniones de Taskapp", "err", msg)
	}
}
