package ws

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"smartcity/internal/bl"
	"smartcity/internal/domain"
)

func TestCanSubscribe(t *testing.T) {
	cases := []struct {
		role domain.Role
		ch   string
		want bool
	}{
		{domain.RoleGuest, bl.ChannelBuses, true},
		{domain.RolePassenger, bl.ChannelBuses, true},
		{domain.RoleGuest, bl.ChannelOperatorHeat, false},
		{domain.RolePassenger, bl.ChannelOperatorAlerts, false},
		{domain.RoleOperator, bl.ChannelOperatorHeat, true},
		{domain.RoleOperator, bl.ChannelOperatorAlerts, true},
		{domain.RoleOperator, "random", false},
	}
	for _, c := range cases {
		if got := CanSubscribe(c.role, c.ch); got != c.want {
			t.Errorf("CanSubscribe(%s,%s)=%v want %v", c.role, c.ch, got, c.want)
		}
	}
}

func dial(t *testing.T, srv *httptest.Server) *websocket.Conn {
	t.Helper()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	return conn
}

func TestHubDeliversOnlyToSubscribers(t *testing.T) {
	hub := NewHub()
	role := domain.RoleOperator
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hub.Serve(w, r, role) }))
	defer srv.Close()

	op := dial(t, srv)
	defer op.Close()
	_ = op.WriteJSON(map[string]string{"action": "subscribe", "channel": bl.ChannelOperatorAlerts})
	var m Message
	if err := op.ReadJSON(&m); err != nil || m.Type != "subscribed" {
		t.Fatalf("subscribe ack: %+v %v", m, err)
	}

	idle := dial(t, srv) // connected but not subscribed
	defer idle.Close()
	for hub.ClientCount() < 2 {
		time.Sleep(10 * time.Millisecond)
	}

	if err := hub.Notify(context.Background(), bl.Alert{Type: "anomaly_opened", RouteCode: "65"}); err != nil {
		t.Fatal(err)
	}
	if err := op.ReadJSON(&m); err != nil {
		t.Fatal(err)
	}
	if m.Channel != bl.ChannelOperatorAlerts || m.Type != "alert" {
		t.Fatalf("unexpected %+v", m)
	}
	data := m.Data.(map[string]any)
	if data["routeCode"] != "65" {
		t.Fatalf("payload %+v", data)
	}

	_ = idle.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	if err := idle.ReadJSON(&m); err == nil {
		t.Fatalf("unsubscribed client received %+v", m)
	}
}
