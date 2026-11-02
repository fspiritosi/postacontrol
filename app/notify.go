package app

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"postacontrol/core/model"
)

// EventOpenMessage lo emite la app al frontend cuando el usuario hace clic en
// una notificación. data: {folderId, messageId}; messageId 0 = solo la carpeta.
const EventOpenMessage = "app:open-message"

// NewMailNotification arma la notificación de escritorio de un aviso de correo
// nuevo. El ID es por cuenta: una notificación nueva reemplaza a la anterior.
func NewMailNotification(ev model.NewMail, accountEmail string) notifications.NotificationOptions {
	n := notifications.NotificationOptions{
		ID:       fmt.Sprintf("new-mail-%d", ev.AccountID),
		Subtitle: escapeBody(accountEmail), // en Linux, Wails lo pone dentro del cuerpo con markup
	}
	if ev.Count <= 1 && len(ev.Items) > 0 {
		it := ev.Items[0]
		n.Title = senderName(it)
		n.Body = escapeBody(it.Subject)
		if n.Body == "" {
			n.Body = "(sin asunto)"
		}
		n.Data = map[string]interface{}{"messageId": it.MessageID, "folderId": ev.FolderID}
		return n
	}
	names := make([]string, 0, len(ev.Items))
	for _, it := range ev.Items {
		if name := senderName(it); !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	n.Title = fmt.Sprintf("%d correos nuevos", ev.Count)
	n.Body = escapeBody(strings.Join(names, ", "))
	n.Data = map[string]interface{}{"folderId": ev.FolderID}
	return n
}

// notifKindSendFailed marca en Data la notificación de un envío que falló.
const notifKindSendFailed = "send-failed"

// SendFailedNotification arma la notificación de un envío que falló cuando la
// ventana principal no está a la vista (escondida en la bandeja del sistema o
// minimizada): el aviso de la app sale ahí y no se vería. data es el payload
// de mail:send-finished; false si no corresponde avisar.
func SendFailedNotification(data any, mainVisible bool) (notifications.NotificationOptions, bool) {
	m, ok := data.(map[string]any)
	if !ok || mainVisible {
		return notifications.NotificationOptions{}, false
	}
	if sent, _ := m["ok"].(bool); sent {
		return notifications.NotificationOptions{}, false
	}
	id := toInt64(m["draftId"])
	msg, _ := m["message"].(string)
	body := "Quedó en la Bandeja de salida."
	if msg != "" {
		body = msg + " " + body
	}
	return notifications.NotificationOptions{
		ID:    fmt.Sprintf("send-failed-%d", id),
		Title: "No se pudo enviar el correo",
		Body:  escapeBody(body),
		Data:  map[string]interface{}{"kind": notifKindSendFailed, "draftId": id},
	}, true
}

// notificationOpensMessage dice si tocar la notificación abre un mensaje (la
// de correo nuevo) o solo muestra la ventana (la de un envío que falló).
func notificationOpensMessage(data map[string]interface{}) bool {
	return data["kind"] != notifKindSendFailed
}

// bodyEscaper escapa lo que el cuerpo de una notificación de freedesktop
// interpreta como markup (incluido el subtítulo, que en Linux va dentro del
// cuerpo). El título se muestra tal cual y no se escapa.
var bodyEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

func escapeBody(s string) string { return bodyEscaper.Replace(s) }

func senderName(it model.NewMailItem) string {
	if it.FromName != "" {
		return it.FromName
	}
	return it.FromEmail
}

// ShouldNotify decide si corresponde mostrar la notificación: no si el usuario
// las desactivó, ni si ya está mirando la ventana.
func ShouldNotify(s model.Settings, visible, focused bool) bool {
	if s.DisableNotifications {
		return false
	}
	return !visible || !focused
}

// OpenMessagePayload convierte el UserInfo de la notificación en el payload de
// app:open-message. Acepta int64 (Linux devuelve Data tal cual) y float64
// (lo que sale de un viaje por JSON en otras plataformas).
func OpenMessagePayload(data map[string]interface{}) map[string]any {
	return map[string]any{
		"folderId":  toInt64(data["folderId"]),
		"messageId": toInt64(data["messageId"]),
	}
}

func toInt64(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case int:
		return int64(x)
	case float64:
		return int64(x)
	}
	return 0
}

// desktopNotifier muestra las notificaciones de correo nuevo.
type desktopNotifier struct {
	app     *application.App
	svc     *MailService
	desktop *Desktop
	notif   *notifications.NotificationService
}

// notifyHub es el punto donde wailsNotifier entrega los avisos al
// desktopNotifier, que recién existe después de SetupDesktop. Antes de eso los
// avisos se descartan. Es seguro entre goroutines.
type notifyHub struct {
	p atomic.Pointer[desktopNotifier]
}

func (h *notifyHub) newMail(ev model.NewMail) {
	if h == nil {
		return
	}
	if d := h.p.Load(); d != nil {
		// Emit no puede volver a llamar al núcleo (contrato de mail.Notifier).
		go d.handle(ev)
	}
}

func (h *notifyHub) sendFinished(data any) {
	if h == nil {
		return
	}
	if d := h.p.Load(); d != nil {
		go d.handleSendFinished(data)
	}
}

// SetupNotifications conecta las notificaciones de escritorio: correo nuevo
// con la ventana escondida o sin foco, y clic que abre el mensaje; un envío
// que falló con la ventana escondida, y clic que la muestra.
func SetupNotifications(app *application.App, svc *MailService, d *Desktop, notif *notifications.NotificationService) {
	n := &desktopNotifier{app: app, svc: svc, desktop: d, notif: notif}
	notif.OnNotificationResponse(func(r notifications.NotificationResult) {
		if r.Error != nil || r.Response.ActionIdentifier != notifications.DefaultActionIdentifier {
			return
		}
		n.desktop.ShowWindow()
		if notificationOpensMessage(r.Response.UserInfo) {
			n.app.Event.Emit(EventOpenMessage, OpenMessagePayload(r.Response.UserInfo))
		}
	})
	svc.hub.p.Store(n)
}

func (n *desktopNotifier) handle(ev model.NewMail) {
	core := n.svc.core
	if core == nil || ev.Count == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// Se lee la configuración en cada aviso (una fila en SQLite): evita una
	// caché que invalidar con mail:settings-changed.
	st, err := core.GetSettings(ctx)
	if err != nil || !ShouldNotify(st, n.desktop.windowVisible(), n.desktop.windowFocused()) {
		return
	}
	email := ""
	if accs, err := core.ListAccounts(ctx); err == nil {
		for _, a := range accs {
			if a.ID == ev.AccountID {
				email = a.Email
			}
		}
	}
	if err := n.notif.SendNotification(NewMailNotification(ev, email)); err != nil {
		slog.Warn("no se pudo mostrar la notificación", "err", err)
	}
}

// handleSendFinished avisa de un envío que falló si la ventana principal no
// está a la vista. No mira DisableNotifications: ese ajuste es del correo
// nuevo, y un correo que no salió no puede pasar inadvertido.
func (n *desktopNotifier) handleSendFinished(data any) {
	opts, ok := SendFailedNotification(data, n.desktop.windowVisible())
	if !ok {
		return
	}
	if err := n.notif.SendNotification(opts); err != nil {
		slog.Warn("no se pudo mostrar la notificación", "err", err)
	}
}
