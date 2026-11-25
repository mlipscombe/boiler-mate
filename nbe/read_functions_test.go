package nbe

import (
	"errors"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"
)

func formatError(t *testing.T, err error, function Function) {
	t.Helper()
	var format *ReplyFormatError
	if !errors.As(err, &format) || format.Function != function {
		t.Fatalf("error = %v, want a format error for function %d", err, function)
	}
}

func TestCustomRequest(t *testing.T) {
	names := []string{"boiler_temp", "state", "state_text"}
	payload, err := CustomRequestPayload(names)
	if err != nil || payload != "boiler_temp,state,state_text" {
		t.Fatalf("payload = %q, %v", payload, err)
	}
	for _, invalid := range [][]string{nil, {""}, {"a,b"}, {"a;b"}, {"a=b"}, {"a b"}, {"a", "a"}, {strings.Repeat("x", 1000)}} {
		if _, err := CustomRequestPayload(invalid); err == nil {
			t.Errorf("accepted names %q", invalid)
		}
	}

	want := map[string]any{"boiler_temp": RoundedFloat(62.5), "state": int64(5), "state_text": "Power"}
	for _, reply := range []string{"62.5,5,Power", "62.5,5,Power,", "62.5, 5 ,Power;", " 62.5,5,Power\n"} {
		values, err := ParseCustomValues(names, reply)
		if err != nil || !reflect.DeepEqual(values, want) {
			t.Errorf("reply %q = %v, %v", reply, values, err)
		}
	}
	// A count mismatch must not pair anything: values would shift by one name.
	for _, reply := range []string{"", ";", "62.5,5", "62.5,5,Power,1", "62.5,5,Power,,"} {
		values, err := ParseCustomValues(names, reply)
		if values != nil {
			t.Errorf("reply %q paired %v", reply, values)
		}
		formatError(t, err, CustomRequestFunction)
	}
}

func TestEventLog(t *testing.T) {
	zone := time.FixedZone("controller", 3600)
	at := time.Date(2024, 11, 24, 9, 5, 7, 0, zone)
	if request := EventLogRequest(at); request != "241124-090507" {
		t.Fatalf("request = %q", request)
	}

	events, err := ParseEventLog("241124,101500,0,12,60,65,C;241124,093000,4,13,0,0;", zone)
	want := []ControllerEvent{
		{Date: "241124", Clock: "101500", At: time.Date(2024, 11, 24, 10, 15, 0, 0, zone), Type: 0, Fields: [3]string{"12", "60", "65"}, Unit: "C"},
		{Date: "241124", Clock: "093000", At: time.Date(2024, 11, 24, 9, 30, 0, 0, zone), Type: 4, Fields: [3]string{"13", "0", "0"}},
	}
	if err != nil || !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %+v, %v", events, err)
	}

	for _, empty := range []string{"", ";", ";;", "*", "*;", "241124,101500,0,*,0,0;"} {
		events, err := ParseEventLog(empty, nil)
		if err != nil || events == nil || len(events) != 0 {
			t.Errorf("reply %q = %v, %v", empty, events, err)
		}
	}

	// Short records and a non-numeric type are skipped; the valid one is kept.
	events, err = ParseEventLog("241124,101500,0,12,60;241124,101500,x,1,2,3;bad;241124,093000,7,1,2,3", zone)
	if err != nil || len(events) != 1 || events[0].Type != 7 {
		t.Fatalf("events = %+v, %v", events, err)
	}
	// An unreadable date keeps the record but leaves its time unset.
	events, err = ParseEventLog("24112,1015,1,a,b,c", zone)
	if err != nil || len(events) != 1 || !events[0].At.IsZero() || events[0].Date != "24112" {
		t.Fatalf("events = %+v, %v", events, err)
	}
	_, err = ParseEventLog("241124,101500,0,12,60;garbage", zone)
	formatError(t, err, GetEventLogFunction)
}

func TestChartRequestPayload(t *testing.T) {
	zone := time.FixedZone("controller", 3600)
	from := time.Date(2024, 11, 24, 9, 0, 0, 0, zone)
	to := time.Date(2024, 11, 24, 10, 0, 0, 0, zone)
	for _, tc := range []struct {
		request ChartRequest
		want    string
	}{
		// The app sends the day, then the end of the interval, then its start.
		{ChartRequest{From: from, To: to, IDs: []int{3}}, "241124,100000,090000,3;"},
		{ChartRequest{From: from, To: to, Step: 1, IDs: []int{3, 7}}, "241124,100000,090000,3,7;"},
		{ChartRequest{From: from, To: to, Step: 5, IDs: []int{0, 12}}, "241124,100000,090000,step=5,0,12;"},
		{ChartRequest{From: from, To: to, IDs: []int{3}, StartFirst: true}, "241124,090000,100000,3;"},
		{ChartRequest{From: to, To: to, IDs: []int{3}}, "241124,100000,100000,3;"},
		// The day is taken in the end time's zone.
		{ChartRequest{From: from.UTC(), To: to, IDs: []int{3}}, "241124,100000,090000,3;"},
	} {
		payload, err := tc.request.Payload()
		if err != nil || payload != tc.want {
			t.Errorf("%+v = %q, %v, want %q", tc.request, payload, err, tc.want)
		}
	}
	for _, invalid := range []ChartRequest{
		{},
		{From: from, To: to},
		{From: to, To: from, IDs: []int{3}},
		{From: from.AddDate(0, 0, -1), To: to, IDs: []int{3}},
		{From: from, To: to, Step: -1, IDs: []int{3}},
		{From: from, To: to, IDs: []int{-1}},
		{From: from, To: to, IDs: []int{7, 3}},
		{From: from, To: to, IDs: []int{3, 3}},
		{From: from, To: to, IDs: make([]int, 600)},
	} {
		if payload, err := invalid.Payload(); err == nil {
			t.Errorf("accepted %+v as %q", invalid, payload)
		}
	}
}

func TestParseChartData(t *testing.T) {
	zone := time.FixedZone("controller", 3600)
	to := time.Date(2024, 11, 24, 10, 0, 0, 0, zone)
	request := ChartRequest{From: to.Add(-time.Hour), To: to, Step: 5, IDs: []int{3, 7}}

	samples, err := ParseChartData("100000,61.5,12;60,61.0,11.5;time=120,60.5,11;,60,10.5;", request)
	if err != nil || len(samples) != 4 {
		t.Fatalf("samples = %+v, %v", samples, err)
	}
	for index, want := range []struct {
		raw    string
		at     time.Time
		values []float64
	}{
		{"100000", to, []float64{61.5, 12}},
		{"60", to.Add(-time.Minute), []float64{61, 11.5}},
		{"time=120", to.Add(-3 * time.Minute), []float64{60.5, 11}},
		{"", to.Add(-8 * time.Minute), []float64{60, 10.5}},
	} {
		got := samples[index]
		if got.RawTime != want.raw || !got.At.Equal(want.at) || !reflect.DeepEqual(got.Values, want.values) {
			t.Errorf("sample %d = %+v, want %+v", index, got, want)
		}
	}

	// Relative times have nothing to count back from until a clock time arrives.
	samples, err = ParseChartData("60,1,2;time=x,1,2;093000,1,2;30,1,2;999999,1,2;15,1,2", request)
	if err != nil || len(samples) != 6 {
		t.Fatalf("samples = %+v, %v", samples, err)
	}
	for index, resolved := range []bool{false, false, true, true, false, false} {
		if samples[index].At.IsZero() == resolved {
			t.Errorf("sample %d time = %v, want resolved %t", index, samples[index].At, resolved)
		}
	}

	for _, empty := range []string{"", ";", " ;; "} {
		samples, err := ParseChartData(empty, request)
		if err != nil || samples == nil || len(samples) != 0 {
			t.Errorf("reply %q = %v, %v", empty, samples, err)
		}
	}
	// Short records and unreadable values are skipped; extra values are ignored.
	samples, err = ParseChartData("100000,1;100000,1,x;100000,1,NaN;095900,1,2,3", request)
	if err != nil || len(samples) != 1 || samples[0].RawTime != "095900" || len(samples[0].Values) != 2 {
		t.Fatalf("samples = %+v, %v", samples, err)
	}
	_, err = ParseChartData("100000,1;nonsense", request)
	formatError(t, err, GetChartDataFunction)
	_, err = ParseChartData("100000,1,2", ChartRequest{})
	formatError(t, err, GetChartDataFunction)
}

func TestParseAvailablePrograms(t *testing.T) {
	want := []ProgramVersion{{"150203", "0616", "0"}, {"150410", "0702", "1"}}
	for _, reply := range []string{"150203,0616,0;150410,0702,1", "150203,0616,0;150410,0702,1;", "150203,0616,0;;150410,0702,1,extra;"} {
		versions, err := ParseAvailablePrograms(reply)
		if err != nil || !reflect.DeepEqual(versions, want) {
			t.Errorf("reply %q = %+v, %v", reply, versions, err)
		}
	}
	for _, empty := range []string{"", ";"} {
		versions, err := ParseAvailablePrograms(empty)
		if err != nil || versions == nil || len(versions) != 0 {
			t.Errorf("reply %q = %v, %v", empty, versions, err)
		}
	}
	versions, err := ParseAvailablePrograms("150203,0616;150410,0702,1")
	if err != nil || len(versions) != 1 || versions[0].Version != "0702" {
		t.Fatalf("versions = %+v, %v", versions, err)
	}
	_, err = ParseAvailablePrograms("150203,0616;150410")
	formatError(t, err, GetAvailableProgramsFunction)
}

func TestParseRemoteSensors(t *testing.T) {
	sensors, err := ParseRemoteSensors("00:11:22:33:44:55,21.5,3.1,3600,42,0,128;66:77:88:99:AA:BB,-4,2.9,3600,300;AA:BB,1,2,3,4,x;CC:DD,1,2,3,4,x,many;")
	if err != nil || len(sensors) != 4 {
		t.Fatalf("sensors = %+v, %v", sensors, err)
	}
	first := sensors[0]
	if first.MAC != "00:11:22:33:44:55" || first.Temperature != 21.5 || first.Battery != 3.1 || first.Timeout != 3600 ||
		first.SecondsSinceReading != 42 || first.Unknown != "0" || first.TransmissionsSent == nil || *first.TransmissionsSent != 128 {
		t.Errorf("first sensor = %+v", first)
	}
	if second := sensors[1]; second.Temperature != -4 || second.Unknown != "" || second.TransmissionsSent != nil {
		t.Errorf("second sensor = %+v", second)
	}
	if sensors[2].Unknown != "x" || sensors[2].TransmissionsSent != nil || sensors[3].TransmissionsSent != nil {
		t.Errorf("optional fields = %+v, %+v", sensors[2], sensors[3])
	}

	for _, empty := range []string{"", ";"} {
		sensors, err := ParseRemoteSensors(empty)
		if err != nil || sensors == nil || len(sensors) != 0 {
			t.Errorf("reply %q = %v, %v", empty, sensors, err)
		}
	}
	// Short records and unreadable numbers are skipped.
	sensors, err = ParseRemoteSensors("AA,1,2,3;BB,warm,2,3,4;CC,1,Inf,3,4;DD,1,2,3.5,4;,1,2,3,4;EE,1,2,3,4")
	if err != nil || len(sensors) != 1 || sensors[0].MAC != "EE" {
		t.Fatalf("sensors = %+v, %v", sensors, err)
	}
	_, err = ParseRemoteSensors("AA,1,2,3")
	formatError(t, err, GetRemoteSensorsFunction)
}

func openMock(t *testing.T) (*MockBoiler, *NBE) {
	t.Helper()
	mock, err := NewMockBoiler("SIM123")
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mock.Stop)
	client, err := Open(t.Context(), &url.URL{Scheme: "udp", Host: mock.GetAddr()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	return mock, client
}

func TestMockBoilerReadFunctions(t *testing.T) {
	mock, client := openMock(t)
	ctx := t.Context()

	// Function 11 must agree with function 4 for the same names.
	operating, err := client.Get(ctx, GetOperatingDataFunction, "*")
	if err != nil {
		t.Fatal(err)
	}
	values, err := client.CustomRequest(ctx, "boiler_temp", "state")
	if err != nil || len(values) != 2 || values["boiler_temp"] != operating.Payload["boiler_temp"] || values["state"] != operating.Payload["state"] {
		t.Errorf("custom values = %v, %v, want those of %v", values, err, operating.Payload)
	}

	events, err := client.EventLog(ctx, time.Now())
	if err != nil || len(events) != 3 || events[0].Type != 0 || events[0].Fields != [3]string{"12", "60", "65"} || events[0].At.IsZero() {
		t.Errorf("events = %+v, %v", events, err)
	}

	to := time.Date(2024, 11, 24, 10, 0, 0, 0, time.Local)
	samples, err := client.ChartData(ctx, ChartRequest{From: to.Add(-time.Hour), To: to, Step: 5, IDs: []int{3, 7}})
	if err != nil || len(samples) != 3 || !reflect.DeepEqual(samples[0].Values, []float64{3, 7}) ||
		!samples[0].At.Equal(to) || !samples[2].At.Equal(to.Add(-2*time.Minute)) {
		t.Errorf("samples = %+v, %v", samples, err)
	}

	versions, err := client.AvailablePrograms(ctx)
	if err != nil || len(versions) != 2 || versions[0] != (ProgramVersion{"150203", "0616", "0"}) {
		t.Errorf("versions = %+v, %v", versions, err)
	}

	sensors, err := client.RemoteSensors(ctx)
	if err != nil || len(sensors) != 2 || sensors[0].MAC != "00:11:22:33:44:55" || sensors[1].TransmissionsSent != nil {
		t.Errorf("sensors = %+v, %v", sensors, err)
	}

	if mock.WriteAttempts() != 0 {
		t.Fatal("a read function sent a settings write")
	}
}

func TestMockBoilerRejectedAndMalformedReplies(t *testing.T) {
	mock, client := openMock(t)
	ctx := t.Context()
	to := time.Date(2024, 11, 24, 10, 0, 0, 0, time.Local)
	chart := ChartRequest{From: to.Add(-time.Hour), To: to, IDs: []int{3}}
	calls := map[Function]func() error{
		CustomRequestFunction:        func() error { _, err := client.CustomRequest(ctx, "boiler_temp"); return err },
		GetEventLogFunction:          func() error { _, err := client.EventLog(ctx, to); return err },
		GetChartDataFunction:         func() error { _, err := client.ChartData(ctx, chart); return err },
		GetAvailableProgramsFunction: func() error { _, err := client.AvailablePrograms(ctx); return err },
		GetRemoteSensorsFunction:     func() error { _, err := client.RemoteSensors(ctx); return err },
	}

	// The mock rejects a name it does not know.
	var rejected *ResponseError
	if _, err := client.CustomRequest(ctx, "no_such_value"); !errors.As(err, &rejected) {
		t.Errorf("unknown custom name error = %v", err)
	}
	for function, call := range calls {
		mock.SetRawReply(function, 3, "ignored")
		var rejected *ResponseError
		if err := call(); !errors.As(err, &rejected) || rejected.Status != 3 || rejected.Function != function {
			t.Errorf("function %d status error = %v", function, err)
		}
		mock.SetRawReply(function, 0, "not,what;the app expects")
		formatError(t, call(), function)
	}
	// An empty list is not an error for the record functions.
	for _, function := range []Function{GetEventLogFunction, GetChartDataFunction, GetAvailableProgramsFunction, GetRemoteSensorsFunction} {
		mock.SetRawReply(function, 0, "")
		if err := calls[function](); err != nil {
			t.Errorf("function %d empty reply error = %v", function, err)
		}
	}
	if mock.WriteAttempts() != 0 {
		t.Fatal("a read function sent a settings write")
	}
}
