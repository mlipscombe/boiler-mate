package heating

import (
	"reflect"
	"testing"
	"time"
)

func TestZoneDemandObservesMQTTEdgesAndLoss(t *testing.T) {
	control := newTestControl(t, defaultSettings())
	control.dashboardReadings = true
	control.Receive(north, []byte(`{"action_topic":"north/action","availability_topic":"north/availability"}`), true)
	var observed []map[string]bool
	control.WatchZoneDemand([]string{north}, func(_ time.Time, states map[string]bool) {
		observed = append(observed, states)
	})
	control.Receive("north/action", []byte("heating"), true)
	control.Receive("north/availability", []byte("online"), true)
	control.Receive("north/action", []byte("heating"), false)
	control.Receive("north/action", []byte("idle"), false)
	control.Receive("north/availability", []byte("offline"), false)
	control.Receive("north/availability", []byte("online"), false)
	control.Receive("north/action", []byte("heating"), false)
	control.Disconnected()
	want := []map[string]bool{{north: true}, {north: false}, {}, {north: true}, {}}
	if !reflect.DeepEqual(observed, want) {
		t.Fatalf("zone demand observations = %v, want %v", observed, want)
	}
}
