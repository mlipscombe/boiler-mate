package webui

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"time"
	_ "time/tzdata" // Calendar reports need IANA zones in the scratch container.
	"unicode/utf8"
)

var (
	pelletPurchaseID = regexp.MustCompile(`^[a-f0-9]{32}$`)
	pelletCurrency   = regexp.MustCompile(`^[A-Z]{3}$`)
)

func (ui *UI) runPelletCounter(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for ctx.Err() == nil {
		readCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		counter, err := ui.consumption(readCtx, "counter")
		cancel()
		switch {
		case err != nil:
			if ctx.Err() == nil {
				ui.logger.Debug("Could not read the pellet consumption counter", "error", err)
			}
		case len(counter) == 1 && counter[0] >= 0:
			if err := ui.store.recordPelletCounter(ctx, ui.serial, time.Now(), counter[0]); err == nil {
				ui.checkPelletSupply(ctx)
			} else if ctx.Err() == nil {
				ui.logger.Warn("Could not record the pellet consumption counter", "error", err)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (ui *UI) checkPelletSupply(ctx context.Context) {
	created, err := ui.store.checkPelletSupply(ctx, time.Now(), ui.serial)
	if err != nil && ctx.Err() == nil {
		ui.logger.Warn("Could not check pellet supply", "error", err)
	}
	if err == nil && created {
		ui.signalEventChange()
	}
}

func (ui *UI) getPelletLedger(w http.ResponseWriter, r *http.Request) {
	location := time.Local
	if zone := r.URL.Query().Get("tz"); zone != "" {
		var err error
		location, err = time.LoadLocation(zone)
		if err != nil {
			problem(w, http.StatusBadRequest, "Unknown report time zone.")
			return
		}
	}
	now := time.Now().In(location)
	periods := pelletPeriods(now, r.URL.Query().Get("period"))
	if periods == nil {
		problem(w, http.StatusBadRequest, "Choose hour, day, week, month, or year.")
		return
	}
	report, err := ui.store.pelletReport(r.Context(), now, periods)
	if err != nil {
		ui.serverError(w, "read pellet ledger", err)
		return
	}
	writeJSON(w, report)
}

func validPelletUnit(unit string) bool {
	return unit == "kg" || unit == "lb"
}

func (ui *UI) putPelletPurchase(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !pelletPurchaseID.MatchString(id) {
		problem(w, http.StatusBadRequest, "Invalid purchase identifier.")
		return
	}
	var entry pelletPurchase
	if !readJSON(w, r, &entry) {
		return
	}
	entry.Brand = strings.TrimSpace(entry.Brand)
	if entry.At.IsZero() || entry.At.After(time.Now()) || entry.At.Year() < 1970 {
		problem(w, http.StatusBadRequest, "Enter a purchase date and time in the past.")
		return
	}
	if !validPelletUnit(entry.Unit) || entry.Quantity < 0.001 || entry.Quantity > 1000000 || entry.Weight < 0.001 || entry.Weight > 1000000 {
		problem(w, http.StatusBadRequest, "Enter a quantity and weight of at least 0.001 and at most 1,000,000.")
		return
	}
	if entry.Price != nil && (*entry.Price < 0 || *entry.Price > 1000000000) || utf8.RuneCountInString(entry.Brand) > 60 {
		problem(w, http.StatusBadRequest, "Enter a nonnegative price and a brand of up to 60 characters.")
		return
	}
	_, err := ui.store.db.ExecContext(r.Context(), `
		INSERT INTO pellet_purchases (id, purchased_at, quantity, weight, unit, brand, price) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET purchased_at = excluded.purchased_at, quantity = excluded.quantity,
			weight = excluded.weight, unit = excluded.unit, brand = excluded.brand, price = excluded.price
	`, id, entry.At.Unix(), entry.Quantity, entry.Weight, entry.Unit, entry.Brand, entry.Price)
	if err != nil {
		ui.serverError(w, "save pellet purchase", err)
		return
	}
	ui.checkPelletSupply(r.Context())
	w.WriteHeader(http.StatusNoContent)
}

func (ui *UI) deletePelletPurchase(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !pelletPurchaseID.MatchString(id) {
		problem(w, http.StatusBadRequest, "Invalid purchase identifier.")
		return
	}
	if _, err := ui.store.db.ExecContext(r.Context(), "DELETE FROM pellet_purchases WHERE id = ?", id); err != nil {
		ui.serverError(w, "delete pellet purchase", err)
		return
	}
	ui.checkPelletSupply(r.Context())
	w.WriteHeader(http.StatusNoContent)
}

func (ui *UI) putPelletSettings(w http.ResponseWriter, r *http.Request) {
	var settings pelletSettings
	if !readJSON(w, r, &settings) {
		return
	}
	settings.Currency = strings.ToUpper(strings.TrimSpace(settings.Currency))
	if !pelletCurrency.MatchString(settings.Currency) || !validPelletUnit(settings.LowUnit) ||
		settings.LowWeight < 0 || settings.LowWeight > 1000000 {
		problem(w, http.StatusBadRequest, "Enter a three-letter currency code and a valid low-supply threshold.")
		return
	}
	result, err := ui.store.db.ExecContext(r.Context(), `
		UPDATE pellet_settings SET currency = ?, low_weight = ?, low_unit = ?,
			notified = CASE WHEN ? = 0 THEN 0 ELSE notified END
		WHERE id = 1 AND (currency = ? OR NOT EXISTS (SELECT 1 FROM pellet_purchases))
	`, settings.Currency, settings.LowWeight, settings.LowUnit, settings.LowWeight, settings.Currency)
	if err != nil {
		ui.serverError(w, "save pellet settings", err)
		return
	}
	if count, err := result.RowsAffected(); err == nil && count == 0 {
		problem(w, http.StatusConflict, "Currency can be changed while the purchase ledger is empty.")
		return
	}
	ui.checkPelletSupply(r.Context())
	w.WriteHeader(http.StatusNoContent)
}
