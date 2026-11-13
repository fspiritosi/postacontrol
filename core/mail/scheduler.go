package mail

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"postacontrol/core/model"
	"postacontrol/core/store"
)

// Lo que pasa a una hora: los envíos programados (y los pospuestos, ver
// snooze.go). El temporizador revisa cada every; lo vencido mientras la app
// estuvo cerrada sale en la primera vuelta.

const (
	msgUnscheduled  = "Cancelaste la programación."
	msgNotScheduled = "Ese correo ya no está programado."
)

// StartScheduler revisa enseguida y después cada every. No bloquea.
func (c *Core) StartScheduler(ctx context.Context, every time.Duration) {
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
			now := time.Now()
			if err := c.sendDueScheduled(ctx, now); err != nil && ctx.Err() == nil {
				slog.Warn("no se pudieron revisar los envíos programados", "err", err)
			}
			if err := c.returnDueSnoozes(ctx, now); err != nil && ctx.Err() == nil {
				slog.Warn("no se pudieron devolver los correos pospuestos", "err", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	})
}

// sendDueScheduled manda, de a uno, los programados cuya hora llegó a now.
func (c *Core) sendDueScheduled(ctx context.Context, now time.Time) error {
	due, err := c.store.DueScheduledDrafts(ctx, now.Unix())
	if err != nil {
		return err
	}
	for _, d := range due {
		if err := c.ctxErr(ctx); err != nil {
			return err
		}
		q, ok, err := c.claimScheduled(ctx, d)
		if err != nil {
			return err
		}
		if ok {
			_, _ = c.deliver(ctx, q) // el resultado lo avisa EventSendFinished
		}
	}
	return nil
}

// claimScheduled toma el programado para mandarlo. ok es false si ya no
// estaba programado.
func (c *Core) claimScheduled(ctx context.Context, d model.LocalDraft) (queuedSend, bool, error) {
	ok, err := c.store.ClaimScheduled(ctx, d.ID)
	if err != nil || !ok {
		return queuedSend{}, false, err
	}
	c.emit(EventOutboxUpdated, nil)
	acc, err := c.store.GetAccount(ctx, d.Request.AccountID)
	if err != nil {
		// Sin la cuenta no sale: queda en la bandeja con el motivo.
		return queuedSend{}, false, c.failSend(ctx, d.ID, err)
	}
	return queuedSend{id: d.ID, acc: acc, req: d.Request}, true, nil
}

func (c *Core) scheduledDraft(ctx context.Context, id int64) (model.LocalDraft, error) {
	ds, err := c.store.ListLocalDrafts(ctx)
	if err != nil {
		return model.LocalDraft{}, c.ctxOrUserError(ctx, err)
	}
	for _, d := range ds {
		if d.ID == id && d.Status == model.DraftScheduled {
			return d, nil
		}
	}
	return model.LocalDraft{}, &UserError{Code: "notfound", Msg: msgNotScheduled, Err: store.ErrNotFound}
}

// SendScheduledNow manda ya un correo programado, en segundo plano.
func (c *Core) SendScheduledNow(ctx context.Context, id int64) error {
	d, err := c.scheduledDraft(ctx, id)
	if err != nil {
		return err
	}
	q, ok, err := c.claimScheduled(ctx, d)
	if err != nil {
		var ue *UserError
		if errors.As(err, &ue) {
			return nil // ya quedó en la bandeja con el motivo
		}
		return c.ctxOrUserError(ctx, err)
	}
	if !ok {
		return &UserError{Code: "notfound", Msg: msgNotScheduled}
	}
	if !c.spawn(func(life context.Context) { _, _ = c.deliver(life, q) }) {
		return context.Canceled
	}
	return nil
}

// UnscheduleSend cancela la programación: el correo queda en la bandeja de
// salida como no enviado, para editarlo o mandarlo cuando se quiera.
func (c *Core) UnscheduleSend(ctx context.Context, id int64) error {
	ok, err := c.store.UnscheduleLocalDraft(ctx, id, msgUnscheduled)
	if err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	if !ok {
		return &UserError{Code: "notfound", Msg: msgNotScheduled}
	}
	c.emit(EventOutboxUpdated, nil)
	return nil
}
