package mail

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"postacontrol/core/model"
	"postacontrol/core/secrets"
	"postacontrol/core/taskapp"
)

const (
	taskappSecret     = "taskapp-token"
	defaultTaskappAPI = "https://apitaskapp.codecontrol.com.ar"
	defaultTaskappWeb = "https://taskapp.codecontrol.com.ar"
	taskappProjectTTL = 5 * time.Minute
	taskappLoginWait  = 5 * time.Minute
	taskappHTTPWait   = 20 * time.Second
	// taskappUploadWait acota cada subida de un .eml (hasta 20 MB), que no
	// usa el timeout de taskappHTTPWait.
	taskappUploadWait = 5 * time.Minute
	// taskappCreateWait acota el POST que crea la tarea o el ticket, que no
	// se corta si el usuario cancela.
	taskappCreateWait = 60 * time.Second
	// taskappSaveWait acota lo que se guarda en la base después de que
	// Taskapp aceptó algo (y que tampoco se corta si el usuario cancela).
	taskappSaveWait = 30 * time.Second

	msgBadTaskappURL  = "La dirección de Taskapp no es válida."
	msgTaskappAuth    = "Taskapp rechazó la conexión. Volvé a conectar desde Configuración."
	msgTaskappOffline = "No pudimos conectar con Taskapp."
)

// validTaskappURL acepta vacío (usa la de producción) o una URL absoluta
// https; http solo para la propia máquina (los tests y el desarrollo).
func validTaskappURL(s string) bool {
	if s == "" {
		return true
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return false
	}
	switch u.Scheme {
	case "https":
		return true
	case "http":
		h := u.Hostname()
		return h == "127.0.0.1" || h == "localhost"
	}
	return false
}

// taskappURLs devuelve las direcciones de la API y de la web, sin barra final.
func (c *Core) taskappURLs(ctx context.Context) (api, web string, err error) {
	st, err := c.store.GetSettings(ctx)
	if err != nil {
		return "", "", c.ctxOrUserError(ctx, err)
	}
	api, web = strings.TrimRight(st.TaskappAPIURL, "/"), strings.TrimRight(st.TaskappWebURL, "/")
	if api == "" {
		api = defaultTaskappAPI
	}
	if web == "" {
		web = defaultTaskappWeb
	}
	return api, web, nil
}

// TaskappWebURL es la dirección de la web de Taskapp configurada (o la de
// siempre), sin barra final.
func (c *Core) TaskappWebURL(ctx context.Context) string {
	_, web, err := c.taskappURLs(ctx)
	if err != nil {
		return defaultTaskappWeb
	}
	return web
}

func (c *Core) taskappHTTP() *http.Client {
	if c.cfg.TaskappHTTP != nil {
		return c.cfg.TaskappHTTP
	}
	return &http.Client{Timeout: taskappHTTPWait}
}

// taskappUploadHTTP es el cliente de las subidas: el mismo transporte que
// taskappHTTP pero sin su Timeout (las acota taskappUploadWait por contexto).
func (c *Core) taskappUploadHTTP() *http.Client {
	base := c.taskappHTTP()
	return &http.Client{Transport: base.Transport, CheckRedirect: base.CheckRedirect, Jar: base.Jar}
}

// forgetTaskapp olvida el usuario y los proyectos en memoria.
func (c *Core) forgetTaskapp() {
	c.taskappMu.Lock()
	c.taskappMe, c.taskappProjs, c.taskappProjAt = nil, nil, time.Time{}
	c.taskappMu.Unlock()
}

// taskURL es la página de la tarea (o del ticket) en la web de Taskapp.
func (c *Core) taskURL(id int64, ticket bool) string {
	_, web, err := c.taskappURLs(context.Background())
	if err != nil {
		web = defaultTaskappWeb
	}
	kind := "tasks"
	if ticket {
		kind = "tickets"
	}
	return web + "/" + kind + "/" + strconv.FormatInt(id, 10)
}

// setTaskappAuthFailed prende o apaga la traba por token rechazado y dice si
// cambió.
func (c *Core) setTaskappAuthFailed(v bool) bool {
	c.taskappMu.Lock()
	defer c.taskappMu.Unlock()
	changed := c.taskappAuthFailed != v
	c.taskappAuthFailed = v
	return changed
}

// taskappClient arma el cliente con el token del llavero. Sin token devuelve
// el error "taskapp-disconnected"; si Taskapp ya rechazó el token, el error
// "taskapp-auth" sin ir a la red (no se reintenta en bucle).
func (c *Core) taskappClient(ctx context.Context) (*taskapp.Client, error) {
	// La dirección antes que el token: SetSettings borra el token antes de
	// guardar una API nueva, así que nunca se junta el token viejo con ella.
	api, _, err := c.taskappURLs(ctx)
	if err != nil {
		return nil, err
	}
	tok, err := c.cfg.Secrets.Get(taskappSecret)
	if errors.Is(err, secrets.ErrNotFound) {
		return nil, &UserError{Code: "taskapp-disconnected", Msg: "Conectá Taskapp en Configuración.", Err: err}
	}
	if err != nil {
		return nil, err
	}
	c.taskappMu.Lock()
	failed := c.taskappAuthFailed
	c.taskappMu.Unlock()
	if failed {
		return nil, &UserError{Code: "taskapp-auth", Msg: msgTaskappAuth, Err: taskapp.ErrUnauthorized}
	}
	return &taskapp.Client{BaseURL: api, Token: tok, HTTP: c.taskappHTTP(), UploadHTTP: c.taskappUploadHTTP()}, nil
}

// taskappErr traduce los errores del cliente de Taskapp. Con 401 además
// descarta lo que había en memoria, traba las próximas llamadas (ver
// taskappClient) y avisa a la UI que hay que reconectar, una sola vez.
func (c *Core) taskappErr(err error) error {
	if errors.Is(err, taskapp.ErrUnauthorized) {
		var ue *UserError
		if !errors.As(err, &ue) {
			c.forgetTaskapp()
			if c.setTaskappAuthFailed(true) {
				c.emit(EventTaskappAuth, nil)
			}
		}
	}
	return taskappUserErr(err)
}

// taskappUserErr traduce los errores del cliente de Taskapp sin efectos.
func taskappUserErr(err error) error {
	if err == nil {
		return nil
	}
	var ue *UserError
	if errors.As(err, &ue) {
		return err
	}
	var api *taskapp.APIError
	switch {
	case errors.Is(err, taskapp.ErrUnauthorized):
		return &UserError{Code: "taskapp-auth", Msg: msgTaskappAuth, Err: err}
	case errors.As(err, &api):
		return &UserError{Code: "taskapp", Msg: "Taskapp: " + api.Msg, Err: err}
	}
	return &UserError{Code: "taskapp-offline", Msg: msgTaskappOffline, Err: err}
}

// isTaskappUnreachable dice si err es de red (y no una respuesta de Taskapp).
func isTaskappUnreachable(err error) bool {
	var api *taskapp.APIError
	return err != nil && !errors.Is(err, taskapp.ErrUnauthorized) && !errors.As(err, &api)
}

func (c *Core) statusOf(u taskapp.User) model.TaskappStatus {
	return model.TaskappStatus{Connected: true, UserName: u.Name, UserEmail: u.Email}
}

// ConnectTaskapp conecta con Taskapp desde el navegador. Una conexión nueva
// cancela la que estuviera esperando. El token se guarda recién cuando Me
// confirma que anda.
func (c *Core) ConnectTaskapp(ctx context.Context, open func(string) error) (model.TaskappStatus, error) {
	ctx, cancel := c.lifeCtx(ctx)
	defer cancel()
	ctx, cancelT := context.WithTimeout(ctx, taskappLoginWait)
	defer cancelT()

	c.taskappMu.Lock()
	if c.connectCancel != nil {
		c.connectCancel()
	}
	c.connectSeq++
	seq := c.connectSeq
	c.connectCancel = cancelT
	c.taskappMu.Unlock()
	defer func() {
		c.taskappMu.Lock()
		if c.connectSeq == seq {
			c.connectCancel = nil
		}
		c.taskappMu.Unlock()
	}()

	api, web, err := c.taskappURLs(ctx)
	if err != nil {
		return model.TaskappStatus{}, err
	}
	grant, err := taskapp.Login(ctx, web, open)
	if err != nil {
		switch {
		case errors.Is(err, taskapp.ErrDenied):
			return model.TaskappStatus{}, &UserError{Code: "taskapp-denied", Msg: "No se autorizó la conexión con Taskapp.", Err: err}
		case errors.Is(err, context.DeadlineExceeded):
			return model.TaskappStatus{}, &UserError{Code: "taskapp-timeout", Msg: "No llegó la autorización de Taskapp. Probá de nuevo.", Err: err}
		}
		return model.TaskappStatus{}, err
	}
	cl := &taskapp.Client{BaseURL: api, Token: grant.Token, HTTP: c.taskappHTTP()}
	me, err := cl.Me(ctx)
	if err != nil {
		if cerr := ctx.Err(); cerr != nil {
			return model.TaskappStatus{}, cerr
		}
		// Un intento fallido: no traba ni avisa (el token guardado, si hay,
		// sigue siendo el de antes).
		return model.TaskappStatus{}, taskappUserErr(err)
	}
	if err := c.cfg.Secrets.Set(taskappSecret, grant.Token); err != nil {
		return model.TaskappStatus{}, err
	}
	c.forgetTaskapp()
	c.taskappMu.Lock()
	c.taskappMe = &me
	c.taskappAuthFailed = false
	c.taskappMu.Unlock()
	c.emit(EventTaskappChanged, nil)
	return c.statusOf(me), nil
}

// DisconnectTaskapp borra el token y lo que se trajo de Taskapp.
func (c *Core) DisconnectTaskapp(ctx context.Context) error {
	if err := c.cfg.Secrets.Delete(taskappSecret); err != nil {
		return err
	}
	c.forgetTaskapp()
	c.setTaskappAuthFailed(false)
	if err := c.store.ClearTaskappData(ctx); err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	c.emit(EventTaskappChanged, nil)
	return nil
}

// TaskappStatus dice si Taskapp está conectada. Sin red, con una conexión ya
// hecha, sigue figurando conectada con el último nombre conocido.
func (c *Core) TaskappStatus(ctx context.Context) (model.TaskappStatus, error) {
	cl, err := c.taskappClient(ctx)
	var ue *UserError
	if errors.As(err, &ue) && ue.Code == "taskapp-disconnected" {
		return model.TaskappStatus{}, nil
	}
	if err != nil {
		return model.TaskappStatus{}, err
	}
	me, err := cl.Me(ctx)
	if err != nil {
		if cerr := c.ctxErr(ctx); cerr != nil {
			return model.TaskappStatus{}, cerr
		}
		if isTaskappUnreachable(err) {
			c.taskappMu.Lock()
			defer c.taskappMu.Unlock()
			st := model.TaskappStatus{Connected: true}
			if c.taskappMe != nil {
				st = c.statusOf(*c.taskappMe)
			}
			return st, nil
		}
		return model.TaskappStatus{}, c.taskappErr(err)
	}
	c.taskappMu.Lock()
	c.taskappMe = &me
	c.taskappMu.Unlock()
	return c.statusOf(me), nil
}

// taskappMeCached devuelve el usuario en memoria o lo pide.
func (c *Core) taskappMeCached(ctx context.Context, cl *taskapp.Client) (taskapp.User, error) {
	c.taskappMu.Lock()
	if c.taskappMe != nil {
		me := *c.taskappMe
		c.taskappMu.Unlock()
		return me, nil
	}
	c.taskappMu.Unlock()
	me, err := cl.Me(ctx)
	if err != nil {
		return taskapp.User{}, err
	}
	c.taskappMu.Lock()
	c.taskappMe = &me
	c.taskappMu.Unlock()
	return me, nil
}

// activeProjects devuelve los proyectos de Taskapp (con caché de 5 minutos).
func (c *Core) activeProjects(ctx context.Context, cl *taskapp.Client) ([]taskapp.Project, error) {
	c.taskappMu.Lock()
	if c.taskappProjs != nil && c.now().Sub(c.taskappProjAt) < taskappProjectTTL {
		ps := slices.Clone(c.taskappProjs)
		c.taskappMu.Unlock()
		return ps, nil
	}
	c.taskappMu.Unlock()
	all, err := cl.Projects(ctx)
	if err != nil {
		return nil, err
	}
	ps := make([]taskapp.Project, 0, len(all))
	for _, p := range all {
		if p.Status == "active" {
			ps = append(ps, p)
		}
	}
	slices.SortStableFunc(ps, func(a, b taskapp.Project) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	c.taskappMu.Lock()
	c.taskappProjs, c.taskappProjAt = slices.Clone(ps), c.now()
	c.taskappMu.Unlock()
	return ps, nil
}

// TaskappProjects devuelve los proyectos activos, ordenados por nombre.
func (c *Core) TaskappProjects(ctx context.Context) ([]model.TaskappProject, error) {
	cl, err := c.taskappClient(ctx)
	if err != nil {
		return nil, err
	}
	ps, err := c.activeProjects(ctx, cl)
	if err != nil {
		if cerr := c.ctxErr(ctx); cerr != nil {
			return nil, cerr
		}
		return nil, c.taskappErr(err)
	}
	out := make([]model.TaskappProject, 0, len(ps))
	for _, p := range ps {
		out = append(out, model.TaskappProject{ID: p.ID, WorkspaceID: p.WorkspaceID, Name: p.Name, Color: p.Color})
	}
	return out, nil
}

// ListTaskappTasks devuelve las tareas y tickets pendientes asignados al
// usuario. Si Taskapp no responde, sirve la última lista guardada (Stale).
func (c *Core) ListTaskappTasks(ctx context.Context) (model.TaskappList, error) {
	cl, err := c.taskappClient(ctx)
	if err != nil {
		return model.TaskappList{}, err
	}
	tasks, err := c.fetchTaskappTasks(ctx, cl)
	if err != nil {
		if cerr := c.ctxErr(ctx); cerr != nil {
			return model.TaskappList{}, cerr
		}
		uerr := c.taskappErr(err)
		var ue *UserError
		if errors.As(uerr, &ue) && ue.Code == "taskapp-auth" {
			return model.TaskappList{}, uerr
		}
		raw, at, ok, cerr := c.store.TaskappCache(ctx)
		if cerr == nil && ok {
			var cached []model.TaskappTask
			if json.Unmarshal([]byte(raw), &cached) == nil {
				if cached == nil {
					cached = []model.TaskappTask{}
				}
				return model.TaskappList{Tasks: cached, FetchedAt: at, Stale: true, Error: uerr.Error()}, nil
			}
		}
		return model.TaskappList{}, uerr
	}
	now := c.now()
	if raw, merr := json.Marshal(tasks); merr == nil {
		// Se avisa solo si la lista cambió: la UI vuelve a pedirla al
		// recibir el evento y no debe entrar en un ciclo.
		old, _, had, _ := c.store.TaskappCache(ctx)
		if serr := c.store.SaveTaskappCache(ctx, string(raw), now); serr != nil {
			return model.TaskappList{}, c.ctxOrUserError(ctx, serr)
		}
		if !had || old != string(raw) {
			c.emit(EventTaskappChanged, nil)
		}
	}
	return model.TaskappList{Tasks: tasks, FetchedAt: now}, nil
}

func (c *Core) fetchTaskappTasks(ctx context.Context, cl *taskapp.Client) ([]model.TaskappTask, error) {
	me, err := c.taskappMeCached(ctx, cl)
	if err != nil {
		return nil, err
	}
	ts, err := cl.ListMyPending(ctx, me.ID)
	if err != nil {
		return nil, err
	}
	ps, err := c.activeProjects(ctx, cl)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]taskapp.Project, len(ps))
	for _, p := range ps {
		byID[p.ID] = p
	}
	out := make([]model.TaskappTask, 0, len(ts))
	for _, t := range ts {
		p := byID[t.ProjectID]
		out = append(out, model.TaskappTask{
			ID: t.ID, Ticket: t.Ticket, Title: t.Title, Priority: t.Priority, DueDate: t.DueDate,
			StatusName: t.Status.Name, ProjectID: t.ProjectID, ProjectName: p.Name, ProjectColor: p.Color,
			URL: c.taskURL(t.ID, t.Ticket), Description: t.Description,
		})
	}
	// Orden fijo: la API puede devolver los empates en otro orden y eso
	// dispararía un aviso de cambio que no hubo (ver ListTaskappTasks).
	slices.SortStableFunc(out, func(a, b model.TaskappTask) int {
		return cmp.Or(cmp.Compare(a.ID, b.ID), cmp.Compare(boolInt(a.Ticket), boolInt(b.Ticket)))
	})
	return out, nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
