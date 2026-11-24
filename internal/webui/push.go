package webui

import (
	"context"
	"crypto/ecdh"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"golang.org/x/sync/errgroup"
)

const pushTTL = time.Hour

type webPush struct {
	privateKey string
	publicKey  string
	client     webpush.HTTPClient
}

type pushSubscription struct {
	webpush.Subscription
	id       int64
	kinds    int
	cursor   int64
	version  int64
	failures int
}

func newWebPush(ctx context.Context, store *store) (*webPush, error) {
	push := &webPush{client: &http.Client{
		Timeout: 8 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}}
	readKeys := func() error {
		return store.db.QueryRowContext(ctx, "SELECT private_key, public_key FROM push_keys WHERE id = 1").
			Scan(&push.privateKey, &push.publicKey)
	}
	if err := readKeys(); !errors.Is(err, sql.ErrNoRows) {
		return push, err
	}
	privateKey, publicKey, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		return nil, err
	}
	if _, err := store.db.ExecContext(ctx, "INSERT INTO push_keys VALUES (1, ?, ?) ON CONFLICT DO NOTHING", privateKey, publicKey); err != nil {
		return nil, err
	}
	return push, readKeys()
}

func pushKind(kind string) int {
	switch kind {
	case "alarm":
		return 1
	case "info":
		return 2
	case "connection":
		return 4
	default:
		return 0
	}
}

func validPushEndpoint(endpoint string) bool {
	u, err := url.Parse(endpoint)
	if err != nil || len(endpoint) > 2048 || u.Scheme != "https" || u.Opaque != "" || u.User != nil ||
		u.Fragment != "" || len(u.Path) < 2 || (u.Port() != "" && u.Port() != "443") {
		return false
	}
	host := strings.ToLower(u.Hostname())
	// Browser subscriptions can trigger outbound requests. Only the browser push services are eligible.
	return host == "fcm.googleapis.com" ||
		host == "updates.push.services.mozilla.com" || strings.HasSuffix(host, ".push.services.mozilla.com") ||
		strings.HasSuffix(host, ".push.apple.com") || strings.HasSuffix(host, ".notify.windows.com") ||
		strings.HasSuffix(host, ".wns.windows.com")
}

func validPushKeys(keys webpush.Keys) bool {
	auth, err := base64.RawURLEncoding.DecodeString(keys.Auth)
	if err != nil || len(auth) != 16 {
		return false
	}
	publicKey, err := base64.RawURLEncoding.DecodeString(keys.P256dh)
	if err != nil {
		return false
	}
	_, err = ecdh.P256().NewPublicKey(publicKey)
	return err == nil
}

func (ui *UI) getPushKey(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]string{"publicKey": ui.push.publicKey})
}

func (ui *UI) putPushSubscription(w http.ResponseWriter, r *http.Request) {
	var input struct {
		webpush.Subscription
		Kinds []string `json:"kinds"`
	}
	if !readJSON(w, r, &input) {
		return
	}
	if !validPushEndpoint(input.Endpoint) || !validPushKeys(input.Keys) {
		problem(w, http.StatusBadRequest, "This browser’s push subscription is invalid or unsupported.")
		return
	}
	var kinds int
	for _, kind := range input.Kinds {
		bit := pushKind(kind)
		if bit == 0 {
			problem(w, http.StatusBadRequest, "Unknown notification category.")
			return
		}
		kinds |= bit
	}
	_, err := ui.store.db.ExecContext(r.Context(), `
		INSERT INTO push_subscriptions(endpoint, auth, p256dh, kinds, cursor)
		VALUES (?, ?, ?, ?, (SELECT COALESCE(MAX(id), 0) FROM events))
		ON CONFLICT(endpoint) DO UPDATE SET auth = excluded.auth, p256dh = excluded.p256dh,
			kinds = excluded.kinds, version = version + 1, retry_at = 0, failures = 0
		WHERE auth <> excluded.auth OR p256dh <> excluded.p256dh OR kinds <> excluded.kinds
	`, input.Endpoint, input.Keys.Auth, input.Keys.P256dh, kinds)
	if err != nil {
		ui.serverError(w, "save push subscription", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (ui *UI) deletePushSubscription(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Endpoint string `json:"endpoint"`
	}
	if !readJSON(w, r, &input) {
		return
	}
	if _, err := ui.store.db.ExecContext(r.Context(), "DELETE FROM push_subscriptions WHERE endpoint = ?", input.Endpoint); err != nil {
		ui.serverError(w, "remove push subscription", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (ui *UI) runWebPush(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for ctx.Err() == nil {
		changed := ui.eventChanges()
		if err := ui.dispatchWebPush(ctx); err != nil && ctx.Err() == nil {
			ui.logger.Warn("Could not process desktop notifications", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-changed:
		case <-ticker.C:
		}
	}
}

func (ui *UI) dispatchWebPush(ctx context.Context) error {
	subscriptions, err := ui.store.duePushSubscriptions(ctx, time.Now())
	if err != nil {
		return err
	}
	var workers errgroup.Group
	workers.SetLimit(4)
	for _, subscription := range subscriptions {
		workers.Go(func() error { return ui.deliverWebPush(ctx, subscription) })
	}
	return workers.Wait()
}

func (s *store) duePushSubscriptions(ctx context.Context, now time.Time) ([]pushSubscription, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, endpoint, auth, p256dh, kinds, cursor, version, failures
		FROM push_subscriptions
		WHERE retry_at <= ? AND EXISTS (
			SELECT 1 FROM events WHERE id > cursor AND seen = 0 AND kind IN ('alarm', 'info', 'connection')
		)
	`, now.Unix())
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var subscriptions []pushSubscription
	for rows.Next() {
		var sub pushSubscription
		if err := rows.Scan(&sub.id, &sub.Endpoint, &sub.Keys.Auth, &sub.Keys.P256dh, &sub.kinds, &sub.cursor, &sub.version, &sub.failures); err != nil {
			return nil, err
		}
		subscriptions = append(subscriptions, sub)
	}
	return subscriptions, rows.Err()
}

func (ui *UI) deliverWebPush(ctx context.Context, sub pushSubscription) error {
	// Include skipped categories in the page so the cursor advances past them too.
	notices, err := ui.store.desktopNoticesAfter(ctx, sub.cursor)
	if err != nil {
		return err
	}
	for _, notice := range notices {
		remaining := time.Until(time.Unix(notice.Time, 0).Add(pushTTL))
		if remaining > 0 && pushKind(notice.Kind)&sub.kinds != 0 {
			status, retryAfter := ui.push.send(ctx, sub, notice, remaining)
			switch {
			case ctx.Err() != nil:
				return ctx.Err()
			case status == http.StatusNotFound || status == http.StatusGone:
				_, err := ui.store.db.ExecContext(ctx, "DELETE FROM push_subscriptions WHERE id = ? AND version = ?", sub.id, sub.version)
				return err
			case status < 200 || status >= 300:
				delay := max(retryAfter, 5*time.Second<<min(sub.failures, 8))
				_, err := ui.store.db.ExecContext(ctx, `
					UPDATE push_subscriptions SET failures = failures + 1, retry_at = ?
					WHERE id = ? AND version = ?
				`, time.Now().Add(delay).Unix(), sub.id, sub.version)
				ui.logger.Warn("Desktop notification delivery failed; will retry", "status", status)
				return err
			}
		}
		result, err := ui.store.db.ExecContext(ctx, `
			UPDATE push_subscriptions SET cursor = ?, failures = 0, retry_at = 0
			WHERE id = ? AND version = ?
		`, notice.ID, sub.id, sub.version)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil || count == 0 {
			return err
		}
		sub.failures = 0
	}
	return nil
}

func (p *webPush) send(ctx context.Context, sub pushSubscription, notice event, remaining time.Duration) (int, time.Duration) {
	body := []rune(notice.Message)
	payload, err := json.Marshal(struct {
		ID   int64  `json:"id"`
		Body string `json:"body"`
	}{notice.ID, string(body[:min(len(body), 240)])})
	if err != nil {
		return 0, 0
	}
	urgency := webpush.UrgencyNormal
	if notice.Kind == "alarm" {
		urgency = webpush.UrgencyHigh
	}
	response, err := webpush.SendNotificationWithContext(ctx, payload, &sub.Subscription, &webpush.Options{
		HTTPClient: p.client, Subscriber: "https://github.com/mlipscombe/boiler-mate",
		VAPIDPrivateKey: p.privateKey, VAPIDPublicKey: p.publicKey,
		TTL: int(remaining.Seconds()), Topic: "boiler-" + strconv.FormatInt(notice.ID, 10), Urgency: urgency,
	})
	if err != nil {
		return 0, 0
	}
	defer func() { _ = response.Body.Close() }()
	header := response.Header.Get("Retry-After")
	seconds, err := strconv.Atoi(header)
	if err == nil && seconds > 0 {
		return response.StatusCode, time.Duration(min(seconds, 3600)) * time.Second
	}
	if retryAt, err := http.ParseTime(header); err == nil {
		return response.StatusCode, min(time.Until(retryAt), time.Hour)
	}
	return response.StatusCode, 0
}
