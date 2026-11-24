package webui

import (
	"bytes"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
)

type pushRequestFunc func(*http.Request) (*http.Response, error)

func (f pushRequestFunc) Do(r *http.Request) (*http.Response, error) { return f(r) }

func TestWebPushDeliveryWithNoDashboardConnection(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	ui, err := New(ctx, slog.Default(), readOnlyFixture{}, "FIXTURE", dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ui.Close() })
	publicKey := ui.push.publicKey
	browserKey, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sub := webpush.Subscription{
		Endpoint: "https://fcm.googleapis.com/fcm/send/fixture",
		Keys: webpush.Keys{
			P256dh: base64.RawURLEncoding.EncodeToString(browserKey.PublicKey().Bytes()),
			Auth:   base64.RawURLEncoding.EncodeToString([]byte("fixture-auth-key")),
		},
	}
	request := func(method, path string, body any, expected int) *httptest.ResponseRecorder {
		t.Helper()
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(method, path, bytes.NewReader(data))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Boiler-Mate", "1")
		response := httptest.NewRecorder()
		ui.Handler().ServeHTTP(response, req)
		if response.Code != expected {
			t.Fatalf("status %d, want %d: %s", response.Code, expected, response.Body)
		}
		return response
	}
	key := request(http.MethodGet, "/api/notifications/push-key", nil, http.StatusOK)
	if !strings.Contains(key.Body.String(), publicKey) || strings.Contains(key.Body.String(), ui.push.privateKey) {
		t.Fatal("push key endpoint did not expose exactly the public key")
	}
	ui.addEvent(ctx, time.Now(), "alarm", "Before subscribing")
	save := func(subscription webpush.Subscription, kinds []string, code int) {
		t.Helper()
		request(http.MethodPut, "/api/notifications/push", struct {
			webpush.Subscription
			Kinds []string `json:"kinds"`
		}{subscription, kinds}, code)
	}
	bad := sub
	bad.Endpoint = "https://fcm.googleapis.com.example.net/fixture"
	save(bad, []string{"alarm"}, http.StatusBadRequest)
	save(sub, []string{"alarm"}, http.StatusNoContent)
	ui.addEvent(ctx, time.Now(), "info", "Disabled notice")
	ui.addEvent(ctx, time.Now(), "state", "Standby")
	ui.addEvent(ctx, time.Now(), "alarm", "This alarm should arrive")
	save(sub, []string{"alarm"}, http.StatusNoContent)
	if err := ui.Close(); err != nil {
		t.Fatal(err)
	}
	ui, err = New(ctx, slog.Default(), readOnlyFixture{}, "FIXTURE", dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if ui.push.publicKey != publicKey {
		t.Fatal("VAPID identity changed after a restart")
	}
	var topics []string
	status := http.StatusServiceUnavailable
	ui.push.client = pushRequestFunc(func(r *http.Request) (*http.Response, error) {
		t.Helper()
		payload, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, err
		}
		if r.URL.String() != sub.Endpoint || r.Method != http.MethodPost ||
			r.Header.Get("Content-Encoding") != "aes128gcm" ||
			!strings.HasPrefix(r.Header.Get("Authorization"), "vapid ") ||
			!strings.HasSuffix(r.Header.Get("Authorization"), "k="+publicKey) ||
			len(payload) == 0 || bytes.Contains(payload, []byte("This alarm should arrive")) {
			t.Error("expected an encrypted and signed Web Push request")
		}
		topics = append(topics, r.Header.Get("Topic"))
		return &http.Response{StatusCode: status, Header: http.Header{"Retry-After": {"30"}}, Body: http.NoBody}, nil
	})
	dispatch := func() {
		t.Helper()
		if err := ui.dispatchWebPush(ctx); err != nil {
			t.Fatal(err)
		}
	}
	dispatch()
	dispatch()
	if !slices.Equal(topics, []string{"boiler-4"}) {
		t.Fatalf("expected one failed attempt with a delay before retry, got %v", topics)
	}
	if _, err := ui.store.db.ExecContext(ctx, "UPDATE push_subscriptions SET retry_at = 0"); err != nil {
		t.Fatal(err)
	}
	status = http.StatusCreated
	dispatch()
	dispatch()
	if !slices.Equal(topics, []string{"boiler-4", "boiler-4"}) {
		t.Fatalf("expected one retry and no replay after success, got %v", topics)
	}
	save(sub, []string{"connection"}, http.StatusNoContent)
	ui.addEvent(ctx, time.Now(), "alarm", "Newly disabled notice")
	ui.addEvent(ctx, time.Now(), "connection", "Connection lost")
	status = http.StatusGone
	dispatch()
	ui.addEvent(ctx, time.Now(), "connection", "Connection lost again")
	dispatch()
	if !slices.Equal(topics, []string{"boiler-4", "boiler-4", "boiler-6"}) {
		t.Fatalf("expected updated categories and cleanup of expired subscriptions, got %v", topics)
	}
	save(sub, []string{"alarm"}, http.StatusNoContent)
	request(http.MethodDelete, "/api/notifications/push", map[string]string{"endpoint": sub.Endpoint}, http.StatusNoContent)
	ui.addEvent(ctx, time.Now(), "alarm", "After unsubscribing")
	dispatch()
	if len(topics) != 3 {
		t.Fatalf("unsubscribed browser received a notice: %v", topics)
	}
}
