package mail

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"postacontrol/core/htmlx"
	"postacontrol/core/mimex"
	"postacontrol/core/model"
	"postacontrol/core/taskapp"
)

const (
	msgTaskNoProject = "Elegí un proyecto para la tarea."
	msgTaskBadPrio   = "La prioridad no es válida."
	msgMailGone      = "El correo ya no está en esta PC."
	msgLinkGone      = "Ese vínculo ya no existe."
	msgAlreadyAdded  = "Ese correo ya está en la tarea."
)

// taskappEmlLimit es el tamaño desde el cual el .eml se sube sin adjuntos
// (deja margen para los encabezados del multipart). Variable para los tests.
var taskappEmlLimit int64 = taskapp.MaxUpload - 64<<10

var taskPriorities = []string{"none", "low", "medium", "high", "urgent"}

// claim marca la operación key como en curso; si ya lo estaba devuelve un
// error de usuario con msg. release la libera.
func (c *Core) claim(key, msg string) (release func(), err error) {
	c.inflightMu.Lock()
	defer c.inflightMu.Unlock()
	if _, busy := c.inflight[key]; busy {
		return nil, &UserError{Code: "invalid", Msg: msg}
	}
	if c.inflight == nil {
		c.inflight = map[string]struct{}{}
	}
	c.inflight[key] = struct{}{}
	return func() {
		c.inflightMu.Lock()
		delete(c.inflight, key)
		c.inflightMu.Unlock()
	}, nil
}

// taskDescriptionHTML arma la descripción (o el comentario): un encabezado con
// remitente y fecha, y el cuerpo del correo.
func taskDescriptionHTML(d model.MessageDetail, loc *time.Location) string {
	if loc == nil {
		loc = time.Local
	}
	from := html.EscapeString(d.From.Email)
	if d.From.Name != "" {
		from = html.EscapeString(d.From.Name) + " &lt;" + html.EscapeString(d.From.Email) + "&gt;"
	}
	var sb strings.Builder
	sb.WriteString("<p><strong>De:</strong> " + from + " · <strong>Fecha:</strong> " +
		html.EscapeString(d.Date.In(loc).Format("02/01/2006 15:04")) + "</p>")
	if d.HTML != "" {
		// Taskapp lo muestra sin sandbox: política estricta, no la del iframe.
		sb.WriteString(htmlx.SanitizeForExport(d.HTML))
		return sb.String()
	}
	text := strings.ReplaceAll(strings.ReplaceAll(d.Text, "\r\n", "\n"), "\r", "\n")
	for _, para := range strings.Split(text, "\n\n") {
		para = strings.Trim(para, "\n")
		if strings.TrimSpace(para) == "" {
			continue
		}
		lines := strings.Split(para, "\n")
		for i := range lines {
			lines[i] = html.EscapeString(lines[i])
		}
		sb.WriteString("<p>" + strings.Join(lines, "<br>") + "</p>")
	}
	return sb.String()
}

// droppedHTML lista los adjuntos que no se subieron por tamaño.
func droppedHTML(dropped []mimex.DroppedPart) string {
	if len(dropped) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("<p>Adjuntos que no se subieron por tamaño:</p><ul>")
	for _, p := range dropped {
		name := p.Filename
		if name == "" {
			name = "(sin nombre)"
		}
		sb.WriteString("<li>" + html.EscapeString(name) + " (" + formatSize(p.Size) + ")</li>")
	}
	sb.WriteString("</ul>")
	return sb.String()
}

// emlForTask baja el .eml del correo; si es grande lo achica sin adjuntos.
func (c *Core) emlForTask(ctx context.Context, messageID, storedSize int64) ([]byte, []mimex.DroppedPart, error) {
	raw, err := c.rawMessage(ctx, messageID)
	if err != nil {
		return nil, nil, err
	}
	if storedSize > taskappEmlLimit || int64(len(raw)) > taskappEmlLimit {
		stripped, dropped, err := mimex.StripAttachments(raw)
		if err != nil {
			return nil, nil, err
		}
		return stripped, dropped, nil
	}
	return raw, nil, nil
}

func (c *Core) taskProjects(ctx context.Context, cl *taskapp.Client) ([]taskapp.Project, error) {
	ps, err := c.activeProjects(ctx, cl)
	if err != nil {
		if cerr := c.ctxErr(ctx); cerr != nil {
			return nil, cerr
		}
		return nil, c.taskappErr(err)
	}
	return ps, nil
}

// TaskappDraftFor propone título, descripción y proyecto para el correo.
func (c *Core) TaskappDraftFor(ctx context.Context, messageID int64) (model.TaskappDraft, error) {
	cl, err := c.taskappClient(ctx)
	if err != nil {
		return model.TaskappDraft{}, err
	}
	d, err := c.GetMessage(ctx, messageID)
	if err != nil {
		return model.TaskappDraft{}, err
	}
	_, _, size, err := c.store.MessageTaskMeta(ctx, messageID)
	if err != nil {
		return model.TaskappDraft{}, c.ctxOrUserError(ctx, err)
	}
	ps, err := c.taskProjects(ctx, cl)
	if err != nil {
		return model.TaskappDraft{}, err
	}
	title := taskapp.CleanSubject(d.Subject)
	if title == "" {
		title = strings.TrimSpace(d.Subject)
	}
	return model.TaskappDraft{
		MessageID:       messageID,
		Title:           title,
		DescriptionHTML: taskDescriptionHTML(d, time.Local),
		ProjectID:       taskapp.SuggestProject(d.From.Email, ps),
		Oversize:        size > taskappEmlLimit,
	}, nil
}

// CreateTaskappItem crea la tarea o el ticket en Taskapp con el .eml adjunto.
// Si falla algo después de crear, devuelve el vínculo con AttachPending=true
// y sin error: nunca se crea dos veces.
func (c *Core) CreateTaskappItem(ctx context.Context, req model.TaskappCreate) (model.TaskLink, error) {
	req.Title = strings.TrimSpace(req.Title)
	if !req.Ticket && req.ProjectID == 0 {
		return model.TaskLink{}, &UserError{Code: "invalid", Msg: msgTaskNoProject}
	}
	if req.Title == "" {
		return model.TaskLink{}, &UserError{Code: "invalid", Msg: msgTaskNoTitle}
	}
	if req.Priority == "" {
		req.Priority = "none"
	}
	if !slices.Contains(taskPriorities, req.Priority) {
		return model.TaskLink{}, &UserError{Code: "invalid", Msg: msgTaskBadPrio}
	}
	if req.Ticket {
		req.DueDate = ""
	} else if req.DueDate != "" {
		if _, err := time.Parse("2006-01-02", req.DueDate); err != nil {
			return model.TaskLink{}, &UserError{Code: "invalid", Msg: msgTaskBadDate}
		}
	}

	release, err := c.claim(fmt.Sprintf("create:%d", req.MessageID), "Ya se está creando la tarea de este correo.")
	if err != nil {
		return model.TaskLink{}, err
	}
	defer release()
	cl, err := c.taskappClient(ctx)
	if err != nil {
		return model.TaskLink{}, err
	}
	headerID, root, size, err := c.store.MessageTaskMeta(ctx, req.MessageID)
	if errors.Is(err, sql.ErrNoRows) {
		return model.TaskLink{}, &UserError{Code: "not_found", Msg: msgMailGone, Err: err}
	}
	if err != nil {
		return model.TaskLink{}, c.ctxOrUserError(ctx, err)
	}
	d, _, err := c.store.GetMessage(ctx, req.MessageID)
	if err != nil {
		return model.TaskLink{}, c.ctxOrUserError(ctx, err)
	}
	// El .eml se baja antes de crear: si falla, no se crea nada.
	eml, dropped, err := c.emlForTask(ctx, req.MessageID, size)
	if err != nil {
		return model.TaskLink{}, err
	}
	ps, err := c.taskProjects(ctx, cl)
	if err != nil {
		return model.TaskLink{}, err
	}
	var proj taskapp.Project
	for _, p := range ps {
		if p.ID == req.ProjectID {
			proj = p
		}
	}
	desc := htmlx.SanitizeForExport(req.DescriptionHTML) + droppedHTML(dropped)

	var created taskapp.Task
	if req.Ticket {
		created, err = cl.CreateTicket(ctx, taskapp.NewTicket{ProjectID: req.ProjectID, Title: req.Title, Description: desc, Priority: req.Priority})
	} else {
		if proj.ID == 0 {
			return model.TaskLink{}, &UserError{Code: "invalid", Msg: msgTaskNoProject}
		}
		created, err = cl.CreateTask(ctx, taskapp.NewTask{WorkspaceID: proj.WorkspaceID, ProjectID: proj.ID,
			Title: req.Title, Description: desc, Priority: req.Priority, DueDate: req.DueDate})
	}
	if err != nil {
		if cerr := c.ctxErr(ctx); cerr != nil {
			return model.TaskLink{}, cerr
		}
		return model.TaskLink{}, c.taskappErr(err)
	}
	c.emit(EventTaskappChanged, nil)

	// La tarea ya existe en Taskapp: de acá en adelante no se corta si el
	// usuario cancela, para no dejarla sin vínculo y que se cree dos veces.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	var link model.TaskLink
	if c.addLinkHook != nil {
		err = c.addLinkHook()
	}
	if err == nil {
		link, err = c.store.AddTaskLink(ctx, model.TaskLink{ThreadRoot: root, TaskID: created.ID, Ticket: req.Ticket,
			ProjectName: proj.Name, Title: req.Title, AttachPending: true}, headerID)
	}
	if err != nil {
		slog.Warn("taskapp: la tarea se creó pero no se guardó el vínculo", "taskId", created.ID, "err", err)
		return model.TaskLink{}, &UserError{Code: "taskapp-orphan", Err: err,
			Msg: fmt.Sprintf("La tarea #%d se creó en Taskapp, pero no se pudo guardar el vínculo en esta PC. Abrila desde Taskapp; no la vuelvas a crear.", created.ID)}
	}
	link.URL = c.taskURL(link.TaskID, link.Ticket)
	link.AlreadyAdded, link.StateKnown = true, true

	if err := c.attachEml(ctx, cl, link.TaskID, created.Attachments, d.Subject, eml); err != nil {
		return link, nil // queda AttachPending=true para reintentar
	}
	if err := c.store.SetAttachPending(ctx, link.ID, false); err != nil {
		return link, nil
	}
	link.AttachPending = false
	c.emit(EventTaskappChanged, nil)
	return link, nil
}

// attachEml sube el .eml y lo suma a los adjuntos que la tarea ya tenía.
func (c *Core) attachEml(ctx context.Context, cl *taskapp.Client, taskID int64, existing []string, subject string, eml []byte) error {
	key, err := cl.Upload(ctx, emlFilename(subject), eml)
	if err != nil {
		return err
	}
	return cl.SetAttachments(ctx, taskID, append(slices.Clone(existing), key))
}

// RetryTaskLinkAttachment repite solo la subida del .eml de un vínculo que
// quedó con AttachPending. Nunca crea otra tarea.
func (c *Core) RetryTaskLinkAttachment(ctx context.Context, linkID int64) (model.TaskLink, error) {
	release, err := c.claim(fmt.Sprintf("retry:%d", linkID), "Ya se está adjuntando el correo.")
	if err != nil {
		return model.TaskLink{}, err
	}
	defer release()
	link, err := c.store.GetTaskLink(ctx, linkID)
	if errors.Is(err, sql.ErrNoRows) {
		return model.TaskLink{}, &UserError{Code: "not_found", Msg: msgLinkGone, Err: err}
	}
	if err != nil {
		return model.TaskLink{}, c.ctxOrUserError(ctx, err)
	}
	cl, err := c.taskappClient(ctx)
	if err != nil {
		return model.TaskLink{}, err
	}
	src, err := c.store.TaskLinkSource(ctx, linkID)
	if err != nil {
		return model.TaskLink{}, c.ctxOrUserError(ctx, err)
	}
	msgID, err := c.store.MessageIDByHeader(ctx, src)
	if errors.Is(err, sql.ErrNoRows) {
		return model.TaskLink{}, &UserError{Code: "not_found", Msg: msgMailGone, Err: err}
	}
	if err != nil {
		return model.TaskLink{}, c.ctxOrUserError(ctx, err)
	}
	d, _, err := c.store.GetMessage(ctx, msgID)
	if err != nil {
		return model.TaskLink{}, c.ctxOrUserError(ctx, err)
	}
	_, _, size, err := c.store.MessageTaskMeta(ctx, msgID)
	if err != nil {
		return model.TaskLink{}, c.ctxOrUserError(ctx, err)
	}
	eml, _, err := c.emlForTask(ctx, msgID, size)
	if err != nil {
		return model.TaskLink{}, err
	}
	t, err := cl.GetTask(ctx, link.TaskID)
	if err != nil {
		return model.TaskLink{}, c.taskappErrCtx(ctx, err)
	}
	if err := c.attachEml(ctx, cl, link.TaskID, t.Attachments, d.Subject, eml); err != nil {
		return model.TaskLink{}, c.taskappErrCtx(ctx, err)
	}
	if err := c.store.SetAttachPending(ctx, linkID, false); err != nil {
		return model.TaskLink{}, c.ctxOrUserError(ctx, err)
	}
	link.AttachPending = false
	link.URL = c.taskURL(link.TaskID, link.Ticket)
	c.emit(EventTaskappChanged, nil)
	return link, nil
}

// taskappErrCtx es taskappErr, pero si el contexto se canceló devuelve eso.
func (c *Core) taskappErrCtx(ctx context.Context, err error) error {
	if cerr := c.ctxErr(ctx); cerr != nil {
		return cerr
	}
	return c.taskappErr(err)
}

// TaskLinksForMessage devuelve los vínculos del hilo del correo, con la URL,
// si el correo ya se sumó y (si Taskapp responde) si la tarea está completada.
func (c *Core) TaskLinksForMessage(ctx context.Context, messageID int64) ([]model.TaskLink, error) {
	headerID, root, _, err := c.store.MessageTaskMeta(ctx, messageID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &UserError{Code: "not_found", Msg: msgMailGone, Err: err}
	}
	if err != nil {
		return nil, c.ctxOrUserError(ctx, err)
	}
	links, err := c.store.TaskLinksForThread(ctx, root)
	if err != nil {
		return nil, c.ctxOrUserError(ctx, err)
	}
	for i := range links {
		links[i].URL = c.taskURL(links[i].TaskID, links[i].Ticket)
		added, err := c.store.AddedToLink(ctx, links[i].ID, headerID)
		if err != nil {
			return nil, c.ctxOrUserError(ctx, err)
		}
		links[i].AlreadyAdded = added
	}
	cl, err := c.taskappClient(ctx)
	if err != nil {
		return links, nil
	}
	var wg sync.WaitGroup
	for i := range links {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if t, err := cl.GetTask(ctx, links[i].TaskID); err == nil {
				links[i].Completed, links[i].StateKnown = t.Status.Completed, true
			}
		}()
	}
	wg.Wait()
	return links, nil
}

// AddMessageToTask suma el correo a la tarea como comentario interno, con el
// .eml adjunto.
func (c *Core) AddMessageToTask(ctx context.Context, linkID, messageID int64) error {
	release, err := c.claim(fmt.Sprintf("add:%d:%d", linkID, messageID), "Ya se está sumando este correo a la tarea.")
	if err != nil {
		return err
	}
	defer release()
	link, err := c.store.GetTaskLink(ctx, linkID)
	if errors.Is(err, sql.ErrNoRows) {
		return &UserError{Code: "not_found", Msg: msgLinkGone, Err: err}
	}
	if err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	headerID, _, size, err := c.store.MessageTaskMeta(ctx, messageID)
	if errors.Is(err, sql.ErrNoRows) {
		return &UserError{Code: "not_found", Msg: msgMailGone, Err: err}
	}
	if err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	added, err := c.store.AddedToLink(ctx, linkID, headerID)
	if err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	if added {
		return &UserError{Code: "invalid", Msg: msgAlreadyAdded}
	}
	cl, err := c.taskappClient(ctx)
	if err != nil {
		return err
	}
	t, err := cl.GetTask(ctx, link.TaskID)
	if err != nil {
		return c.taskappErrCtx(ctx, err)
	}
	completed := &UserError{Code: "taskapp-completed", Msg: fmt.Sprintf("La tarea #%d está completada.", link.TaskID)}
	if t.Status.Completed {
		return completed
	}
	d, err := c.GetMessage(ctx, messageID)
	if err != nil {
		return err
	}
	eml, dropped, err := c.emlForTask(ctx, messageID, size)
	if err != nil {
		return err
	}
	key, err := cl.Upload(ctx, emlFilename(d.Subject), eml)
	if err != nil {
		return c.taskappErrCtx(ctx, err)
	}
	body := htmlx.SanitizeForExport(taskDescriptionHTML(d, time.Local)) + droppedHTML(dropped)
	if err := cl.AddComment(ctx, link.TaskID, body, []string{key}, true); err != nil {
		var api *taskapp.APIError
		if errors.As(err, &api) && api.Status == 400 && strings.Contains(strings.ToLower(api.Msg), "completed") {
			return completed
		}
		return c.taskappErrCtx(ctx, err)
	}
	if err := c.store.MarkAddedToLink(ctx, linkID, headerID); err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	c.emit(EventTaskappChanged, nil)
	return nil
}

// CreateLocalTaskFrom arma (sin guardar) una tarea local a partir del correo.
func (c *Core) CreateLocalTaskFrom(ctx context.Context, messageID int64) (model.LocalTask, error) {
	d, _, err := c.store.GetMessage(ctx, messageID)
	if errors.Is(err, sql.ErrNoRows) {
		return model.LocalTask{}, &UserError{Code: "not_found", Msg: msgMailGone, Err: err}
	}
	if err != nil {
		return model.LocalTask{}, c.ctxOrUserError(ctx, err)
	}
	headerID, _, _, err := c.store.MessageTaskMeta(ctx, messageID)
	if err != nil {
		return model.LocalTask{}, c.ctxOrUserError(ctx, err)
	}
	title := taskapp.CleanSubject(d.Subject)
	if title == "" {
		title = strings.TrimSpace(d.Subject)
	}
	return model.LocalTask{Title: title, SourceMessageID: headerID, SourceSubject: d.Subject}, nil
}

// FindMessageByHeader busca el correo más nuevo con ese Message-ID.
func (c *Core) FindMessageByHeader(ctx context.Context, headerID string) (model.MessageSummary, error) {
	m, err := c.store.MessageSummaryByHeader(ctx, headerID)
	if errors.Is(err, sql.ErrNoRows) {
		return model.MessageSummary{}, &UserError{Code: "not_found", Msg: msgMailGone, Err: err}
	}
	if err != nil {
		return model.MessageSummary{}, c.ctxOrUserError(ctx, err)
	}
	return m, nil
}
