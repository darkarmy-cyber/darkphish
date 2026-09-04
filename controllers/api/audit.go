package api

import (
	"encoding/csv"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/darkarmy-cyber/darkphish/internal/audit"
	"github.com/darkarmy-cyber/darkphish/models"
)

type auditPage struct {
	Events  []audit.Event `json:"events"`
	Total   int64         `json:"total"`
	Page    int           `json:"page"`
	PerPage int           `json:"per_page"`
}

func parseAuditFilter(r *http.Request) (audit.Filter, error) {
	query := r.URL.Query()
	filter := audit.Filter{
		Action: query.Get("action"), Actor: query.Get("actor"), TargetType: query.Get("target_type"),
		TargetID: query.Get("target_id"), Result: query.Get("result"),
		Page: 1, PerPage: 50,
	}
	var err error
	if value := query.Get("actor_id"); value != "" {
		filter.ActorID, err = strconv.ParseInt(value, 10, 64)
		if err != nil {
			return filter, err
		}
	}
	if value := query.Get("page"); value != "" {
		filter.Page, err = strconv.Atoi(value)
		if err != nil {
			return filter, err
		}
	}
	if value := query.Get("per_page"); value != "" {
		filter.PerPage, err = strconv.Atoi(value)
		if err != nil {
			return filter, err
		}
	}
	if filter.Page < 1 || filter.PerPage < 1 || filter.PerPage > 500 {
		return filter, strconv.ErrSyntax
	}
	for _, value := range []string{filter.Action, filter.Actor, filter.TargetType, filter.TargetID, filter.Result} {
		if len(value) > 255 {
			return filter, strconv.ErrSyntax
		}
	}
	for value, destination := range map[string]**time.Time{"from": &filter.From, "to": &filter.To} {
		if raw := query.Get(value); raw != "" {
			parsed, parseErr := time.Parse(time.RFC3339, raw)
			if parseErr != nil {
				return filter, parseErr
			}
			*destination = &parsed
		}
	}
	return filter, nil
}

func (as *Server) AuditEvents(w http.ResponseWriter, r *http.Request) {
	filter, err := parseAuditFilter(r)
	if err != nil {
		JSONResponse(w, models.Response{Success: false, Message: "Invalid audit filter"}, http.StatusBadRequest)
		return
	}
	events, total, err := audit.Query(filter)
	if err != nil {
		JSONResponse(w, models.Response{Success: false, Message: "Unable to query audit events"}, http.StatusInternalServerError)
		return
	}
	JSONResponse(w, auditPage{Events: events, Total: total, Page: filter.Page, PerPage: filter.PerPage}, http.StatusOK)
}

func safeCSV(value string) string {
	candidate := strings.TrimLeft(value, " \t\r\n")
	if candidate != "" && strings.ContainsRune("=+-@", rune(candidate[0])) {
		return "'" + value
	}
	return value
}

func (as *Server) AuditExport(w http.ResponseWriter, r *http.Request) {
	filter, err := parseAuditFilter(r)
	if err != nil {
		JSONResponse(w, models.Response{Success: false, Message: "Invalid audit filter"}, http.StatusBadRequest)
		return
	}
	filter.Page = 1
	filter.PerPage = 500
	events := make([]audit.Event, 0)
	for len(events) < 10000 {
		page, total, queryErr := audit.Query(filter)
		if queryErr != nil {
			JSONResponse(w, models.Response{Success: false, Message: "Unable to export audit events"}, http.StatusInternalServerError)
			return
		}
		events = append(events, page...)
		if len(events) > 10000 {
			events = events[:10000]
		}
		if int64(len(events)) >= total || len(page) == 0 {
			break
		}
		filter.Page++
	}
	w.Header().Set("Cache-Control", "no-store, max-age=0")
	format := strings.ToLower(r.URL.Query().Get("format"))
	if format == "json" {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", `attachment; filename="darkphish-audit.json"`)
		_ = json.NewEncoder(w).Encode(events)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="darkphish-audit.csv"`)
	writer := csv.NewWriter(w)
	_ = writer.Write([]string{"timestamp", "actor", "actor_id", "actor_type", "action", "target_type", "target_id", "result", "request_id", "source_ip", "auth_method"})
	for _, event := range events {
		_ = writer.Write([]string{
			event.Timestamp.Format(time.RFC3339Nano), safeCSV(event.Actor), strconv.FormatInt(event.ActorID, 10),
			safeCSV(event.ActorType), safeCSV(event.Action), safeCSV(event.TargetType), safeCSV(event.TargetID), safeCSV(event.Result),
			safeCSV(event.RequestID), safeCSV(event.SourceIP), safeCSV(event.AuthMethod),
		})
	}
	writer.Flush()
}
