package webui

import (
	"errors"
	"net/http"

	"github.com/mlipscombe/boiler-mate/internal/controllerread"
	"github.com/mlipscombe/boiler-mate/nbe"
)

func (ui *UI) getControllerRead(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	data, err := ui.read(r.Context(), kind, r.URL.Query())
	if err == nil {
		writeJSON(w, data)
		return
	}
	status := http.StatusServiceUnavailable
	var format *nbe.ReplyFormatError
	switch {
	case errors.Is(err, controllerread.ErrKind):
		status = http.StatusNotFound
	case errors.Is(err, controllerread.ErrQuery):
		status = http.StatusBadRequest
	case errors.As(err, &format):
		status = http.StatusBadGateway
	}
	problem(w, status, controllerread.Message(kind, err))
}
