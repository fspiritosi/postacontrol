package app

import (
	"testing"

	"postacontrol/core/model"
)

func TestNewMailNotification_Single(t *testing.T) {
	cases := []struct {
		name        string
		item        model.NewMailItem
		title, body string
	}{
		{"con nombre", model.NewMailItem{MessageID: 7, FromName: "Ana", FromEmail: "ana@x.com", Subject: "Hola"}, "Ana", "Hola"},
		{"sin nombre", model.NewMailItem{MessageID: 7, FromEmail: "ana@x.com", Subject: "Hola"}, "ana@x.com", "Hola"},
		{"sin asunto", model.NewMailItem{MessageID: 7, FromName: "Ana"}, "Ana", "(sin asunto)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			n := NewMailNotification(model.NewMail{AccountID: 3, FolderID: 9, Count: 1, Items: []model.NewMailItem{c.item}}, "yo@x.com")
			if n.Title != c.title || n.Body != c.body || n.Subtitle != "yo@x.com" || n.ID != "new-mail-3" {
				t.Fatalf("got %+v", n)
			}
			if n.Data["messageId"] != int64(7) || n.Data["folderId"] != int64(9) || len(n.Data) != 2 {
				t.Fatalf("data %+v", n.Data)
			}
		})
	}
}

func TestNewMailNotification_Summary(t *testing.T) {
	n := NewMailNotification(model.NewMail{AccountID: 3, FolderID: 9, Count: 20, Items: []model.NewMailItem{
		{FromName: "Ana"}, {FromName: "Beto"}, {FromEmail: "caro@x.com", FromName: "Caro"},
	}}, "yo@x.com")
	if n.Title != "20 correos nuevos" || n.Body != "Ana, Beto, Caro" || n.Subtitle != "yo@x.com" {
		t.Fatalf("got %+v", n)
	}
	if n.Data["folderId"] != int64(9) || len(n.Data) != 1 {
		t.Fatalf("data %+v", n.Data)
	}
}

func TestNewMailNotification_ReplacesPerAccount(t *testing.T) {
	a := NewMailNotification(model.NewMail{AccountID: 5, Count: 1, Items: []model.NewMailItem{{FromName: "A"}}}, "")
	b := NewMailNotification(model.NewMail{AccountID: 5, Count: 4, Items: []model.NewMailItem{{FromName: "B"}}}, "")
	c := NewMailNotification(model.NewMail{AccountID: 6, Count: 1, Items: []model.NewMailItem{{FromName: "C"}}}, "")
	if a.ID != b.ID || a.ID == c.ID {
		t.Fatalf("ids %q %q %q", a.ID, b.ID, c.ID)
	}
}

func TestShouldNotify(t *testing.T) {
	cases := []struct {
		name            string
		disabled        bool
		visible, focued bool
		want            bool
	}{
		{"desactivadas", true, false, false, false},
		{"visible con foco", false, true, true, false},
		{"escondida", false, false, false, true},
		{"visible sin foco", false, true, false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ShouldNotify(model.Settings{DisableNotifications: c.disabled}, c.visible, c.focued); got != c.want {
				t.Fatalf("got %v", got)
			}
		})
	}
}

func TestOpenMessagePayload(t *testing.T) {
	// En Linux Data vuelve tal cual; en otras plataformas puede volver como JSON (float64).
	for _, d := range []map[string]any{
		{"folderId": int64(9), "messageId": int64(7)},
		{"folderId": float64(9), "messageId": float64(7)},
	} {
		p := OpenMessagePayload(d)
		if p["folderId"] != int64(9) || p["messageId"] != int64(7) {
			t.Fatalf("got %+v", p)
		}
	}
	if p := OpenMessagePayload(map[string]any{"folderId": float64(9)}); p["messageId"] != int64(0) {
		t.Fatalf("got %+v", p)
	}
}
