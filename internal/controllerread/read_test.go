package controllerread

import (
	"context"
	"testing"
	"testing/synctest"

	"github.com/mlipscombe/boiler-mate/nbe"
)

type getFunc func(context.Context, nbe.Function, string) (*nbe.NBEResponse, error)

func (f getFunc) Get(ctx context.Context, fn nbe.Function, payload string) (*nbe.NBEResponse, error) {
	return f(ctx, fn, payload)
}

func TestFirstFirmwareReadRecoversFromDeclinedRequest(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		calls := 0
		controller := getFunc(func(_ context.Context, fn nbe.Function, payload string) (*nbe.NBEResponse, error) {
			calls++
			if fn != nbe.GetAvailableProgramsFunction || payload != "*" {
				t.Fatalf("unexpected request %d %q", fn, payload)
			}
			if calls < 3 {
				return nil, &nbe.ResponseError{Function: fn, Status: 1}
			}
			return &nbe.NBEResponse{RawPayload: "150203,0616,0;"}, nil
		})
		value, err := Read(t.Context(), controller, "versions", nil)
		rows, ok := value.([]nbe.ProgramVersion)
		if err != nil || !ok || len(rows) != 1 || rows[0].Version != "0616" || calls != 3 {
			t.Fatalf("first open returned %v, %v after %d calls", value, err, calls)
		}
	})
}
