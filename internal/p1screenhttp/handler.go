// Package p1screenhttp adapts P1 screen URL helpers to HTTP.
package p1screenhttp

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"wework-go/internal/p1screen"
)

// Handler owns the legacy /api/p1 screen endpoints.
type Handler struct {
	Service p1screen.Service
}

// New builds a P1 screen HTTP adapter.
func New(service p1screen.Service) Handler {
	return Handler{Service: service}
}

// ScreenHTMLHandler serializes GET /api/p1/screen/{slot_index}.
func (handler Handler) ScreenHTMLHandler(w http.ResponseWriter, r *http.Request) {
	slotIndex, ok := slotIndexFromRequest(w, r)
	if !ok {
		return
	}
	quality, ok := qualityFromRequest(w, r)
	if !ok {
		return
	}
	html, err := handler.Service.ScreenHTML(slotIndex, quality)
	if err != nil {
		writeDetail(w, http.StatusBadRequest, err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(html))
}

// ScreenURLHandler serializes GET /api/p1/screen/{slot_index}/url.
func (handler Handler) ScreenURLHandler(w http.ResponseWriter, r *http.Request) {
	slotIndex, ok := slotIndexFromRequest(w, r)
	if !ok {
		return
	}
	quality, ok := qualityFromRequest(w, r)
	if !ok {
		return
	}
	payload, err := handler.Service.ScreenURL(slotIndex, quality)
	if err != nil {
		writeDetail(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// ScreenAPIURLHandler serializes GET /api/p1/screen/{slot_index}/api-url.
func (handler Handler) ScreenAPIURLHandler(w http.ResponseWriter, r *http.Request) {
	slotIndex, ok := slotIndexFromRequest(w, r)
	if !ok {
		return
	}
	quality, ok := qualityFromRequest(w, r)
	if !ok {
		return
	}
	payload, err := handler.Service.APIURL(slotIndex, quality)
	if err != nil {
		writeDetail(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// SlotsPortsHandler serializes GET /api/p1/slots/ports.
func (handler Handler) SlotsPortsHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, handler.Service.SlotsPorts())
}

func slotIndexFromRequest(w http.ResponseWriter, r *http.Request) (int, bool) {
	raw := strings.TrimSpace(r.PathValue("slot_index"))
	slotIndex, err := strconv.Atoi(raw)
	if err != nil {
		writeDetail(w, http.StatusUnprocessableEntity, "slot_index must be an integer")
		return 0, false
	}
	if err := p1screen.ValidateSlot(slotIndex); err != nil {
		writeDetail(w, http.StatusBadRequest, err.Error())
		return 0, false
	}
	return slotIndex, true
}

func qualityFromRequest(w http.ResponseWriter, r *http.Request) (string, bool) {
	quality := p1screen.NormalizeQuality(r.URL.Query().Get("quality"))
	if err := p1screen.ValidateQuality(quality); err != nil {
		writeDetail(w, http.StatusUnprocessableEntity, err.Error())
		return "", false
	}
	return quality, true
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeDetail(w http.ResponseWriter, status int, detail string) {
	writeJSON(w, status, map[string]string{"detail": detail})
}
