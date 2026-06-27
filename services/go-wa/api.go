package main

import (
	"encoding/json"
	"net/http"
	"strings"
)

func apiAuth(key string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Internal-Key") != key {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Unauthorized"})
			return
		}
		next(w, r)
	}
}

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func jsonErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// GET /api/sessions
func handleListSessions(mgr *SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		records, err := dbListSessions(r.Context())
		if err != nil {
			jsonErr(w, 500, err.Error())
			return
		}

		type row struct {
			ID            string `json:"id"`
			Name          string `json:"name"`
			ChatflowID    string `json:"chatflow_id"`
			HumanContact  string `json:"human_contact"`
			AllowPhones   string `json:"allow_phones"`
			DisableUpload bool   `json:"disable_upload"`
			Active        bool   `json:"active"`
			Status        string `json:"status"`
			Phone         string `json:"phone"`
		}

		out := make([]row, 0, len(records))
		for _, rec := range records {
			status := "offline"
			phone := ""
			if s := mgr.Get(rec.ID); s != nil {
				info := s.StatusInfo()
				status = info["status"].(string)
				phone = info["phone"].(string)
			}
			out = append(out, row{
				ID:            rec.ID,
				Name:          rec.Name,
				ChatflowID:    rec.ChatflowID,
				HumanContact:  rec.HumanContact,
				AllowPhones:   rec.AllowPhones,
				DisableUpload: rec.DisableUpload,
				Active:        rec.Active,
				Status:        status,
				Phone:         phone,
			})
		}
		jsonOK(w, out)
	}
}

// POST /api/sessions
func handleCreateSession(mgr *SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name          string `json:"name"`
			ChatflowID    string `json:"chatflow_id"`
			HumanContact  string `json:"human_contact"`
			AllowPhones   string `json:"allow_phones"`
			DisableUpload bool   `json:"disable_upload"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			jsonErr(w, 400, "invalid JSON")
			return
		}
		if strings.TrimSpace(body.Name) == "" || strings.TrimSpace(body.ChatflowID) == "" {
			jsonErr(w, 400, "name and chatflow_id are required")
			return
		}

		rec := &SessionRecord{
			Name:          body.Name,
			ChatflowID:    body.ChatflowID,
			HumanContact:  body.HumanContact,
			AllowPhones:   body.AllowPhones,
			DisableUpload: body.DisableUpload,
		}
		id, err := mgr.Add(r.Context(), rec)
		if err != nil {
			jsonErr(w, 500, err.Error())
			return
		}
		w.WriteHeader(http.StatusCreated)
		jsonOK(w, map[string]string{"id": id})
	}
}

// PUT /api/sessions/{id}
func handleUpdateSession(mgr *SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var body struct {
			Name          string `json:"name"`
			ChatflowID    string `json:"chatflow_id"`
			HumanContact  string `json:"human_contact"`
			AllowPhones   string `json:"allow_phones"`
			DisableUpload bool   `json:"disable_upload"`
			Active        bool   `json:"active"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			jsonErr(w, 400, "invalid JSON")
			return
		}
		rec := &SessionRecord{
			ID:            id,
			Name:          body.Name,
			ChatflowID:    body.ChatflowID,
			HumanContact:  body.HumanContact,
			AllowPhones:   body.AllowPhones,
			DisableUpload: body.DisableUpload,
			Active:        body.Active,
		}
		if err := mgr.Update(r.Context(), rec); err != nil {
			jsonErr(w, 500, err.Error())
			return
		}
		jsonOK(w, map[string]string{"status": "ok"})
	}
}

// DELETE /api/sessions/{id}
func handleDeleteSession(mgr *SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if err := mgr.Remove(r.Context(), id); err != nil {
			jsonErr(w, 500, err.Error())
			return
		}
		jsonOK(w, map[string]string{"status": "deleted"})
	}
}

// GET /api/sessions/{id}/status
func handleSessionStatus(mgr *SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		s := mgr.Get(id)
		if s == nil {
			jsonErr(w, 404, "session not found")
			return
		}
		jsonOK(w, s.StatusInfo())
	}
}

// GET /api/sessions/{id}/qr
func handleSessionQR(mgr *SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		s := mgr.Get(id)
		if s == nil {
			jsonErr(w, 404, "session not found")
			return
		}
		png := s.QR()
		if png == nil {
			jsonErr(w, 404, "no QR available — session may be connected or not started")
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(png)
	}
}

// POST /api/sessions/{id}/connect
func handleSessionConnect(mgr *SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if err := mgr.Connect(r.Context(), id); err != nil {
			jsonErr(w, 404, err.Error())
			return
		}
		jsonOK(w, map[string]string{"status": "connecting"})
	}
}

// POST /api/sessions/{id}/logout
func handleSessionLogout(mgr *SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if err := mgr.Logout(r.Context(), id); err != nil {
			jsonErr(w, 404, err.Error())
			return
		}
		jsonOK(w, map[string]string{"status": "logged_out"})
	}
}

// POST /api/sessions/{id}/pair-phone
func handleSessionPairPhone(mgr *SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		s := mgr.Get(id)
		if s == nil {
			jsonErr(w, 404, "session not found")
			return
		}

		phone := r.URL.Query().Get("phone")
		if phone == "" {
			var body struct {
				Phone string `json:"phone"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			phone = body.Phone
		}
		if phone == "" {
			jsonErr(w, 400, "phone is required")
			return
		}
		// Normalize: remove leading 0, remove +
		phone = strings.TrimPrefix(phone, "+")
		if strings.HasPrefix(phone, "0") {
			phone = "62" + phone[1:]
		}

		code, err := s.PairPhone(r.Context(), phone)
		if err != nil {
			jsonErr(w, 500, err.Error())
			return
		}
		jsonOK(w, map[string]string{"pairing_code": code})
	}
}
