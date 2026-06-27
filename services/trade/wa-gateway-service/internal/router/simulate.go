package router

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

// testCaptures holds per-phone reply accumulators used during /simulate calls.
// Only populated for the duration of a simulate call; nil in production.
var testCaptures sync.Map // map[phone string]*[]string

// captureReply records a reply for a test phone if a capture is active.
// Returns true if captured (caller should skip the real WA send).
func captureReply(phone, text string) bool {
	if v, ok := testCaptures.Load(phone); ok {
		ptr := v.(*[]string)
		*ptr = append(*ptr, text)
		return true
	}
	return false
}

// SimulateRequest is the JSON body for POST /simulate.
type SimulateRequest struct {
	Phone   string `json:"phone"`   // e.g. "628123456789"
	Message string `json:"message"` // plain text body
	Token   string `json:"token"`   // must match QR_TOKEN env var
}

// SimulateResponse is returned from POST /simulate.
type SimulateResponse struct {
	Phone   string   `json:"phone"`
	Message string   `json:"message"`
	Replies []string `json:"replies"`
	Elapsed string   `json:"elapsed"`
}

// makeSimulatedEvent builds a minimal *events.Message for a text message
// from phone so the router can process it as if it came from WhatsApp.
func makeSimulatedEvent(phone, text string) *events.Message {
	senderJID := types.NewJID(phone, types.DefaultUserServer)
	return &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Sender:   senderJID,
				Chat:     senderJID,
				IsFromMe: false,
				IsGroup:  false,
			},
			PushName:  "TestUser",
			Timestamp: time.Now(),
		},
		Message: &waE2E.Message{
			Conversation: proto.String(text),
		},
	}
}

// HandleSimulate is POST /simulate — feeds a fake message through the router
// and returns all replies that would have been sent to the user.
// Protected by the same QR_TOKEN used by the admin endpoints.
func (r *Router) HandleSimulate(qrToken string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var body SimulateRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		if body.Token != qrToken {
			http.Error(w, "invalid token", http.StatusForbidden)
			return
		}
		phone := strings.TrimSpace(body.Phone)
		message := strings.TrimSpace(body.Message)
		if phone == "" || message == "" {
			http.Error(w, "phone and message are required", http.StatusBadRequest)
			return
		}

		// Register capture for this phone.
		replies := make([]string, 0)
		testCaptures.Store(phone, &replies)
		defer testCaptures.Delete(phone)

		start := time.Now()
		evt := makeSimulatedEvent(phone, message)
		if err := r.handleSingleMessage(evt); err != nil {
			http.Error(w, "handler error: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(SimulateResponse{
			Phone:   phone,
			Message: message,
			Replies: replies,
			Elapsed: time.Since(start).String(),
		})
	}
}
