// Package router ports the core message dispatch logic from
// messageHandler.js into wa-gateway-service. The Router struct holds all
// per-instance in-memory state (throttle counters, message queues, active
// conversation contexts) and delegates persistence to state.Store.
package router

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"

	"wa-gateway-service/internal/client"
	"wa-gateway-service/internal/notification"
	"wa-gateway-service/internal/state"
)

// throttle / queue constants mirror messageHandler.js constructor (line 39-43).
const (
	throttleWindow  = 5 * time.Second
	maxMsgPerWindow = 10
	maxQueueSize    = 10
	interMsgDelay   = 500 * time.Millisecond
)

// ActiveConv mirrors this.activeConversations[phone] entries.
type ActiveConv struct {
	State                    string
	BantuanRequested         bool
	WarehouseSearchRequested bool
	LastResults              []client.Product
	Active                   bool
	Context                  string // "product_search"
	LastMessageTime          int64
	Checkout                 *CheckoutData
}

// Router is the Go port of the MessageHandler class. One instance per
// process, shared across all incoming messages.
type Router struct {
	wa      *whatsmeow.Client
	store   *state.Store
	cache   *state.CustomerCache
	ai      *client.AIVisionClient
	flowise      *client.FlowiseClient // optional; nil = use Gemini only
	ownerFlowise *client.FlowiseClient // optional; owner-assistant chatflow
	ownerPhones  map[string]bool       // set of phone numbers with owner access
	search       *client.ProductSearchClient

	catalogExportURL string // product-search-service /export/csv endpoint

	notif            *notification.Notifier
	marketingNumbers []string

	mu          sync.Mutex
	lastMsgTime map[string]int64
	msgCount    map[string]int
	queues      map[string][]*events.Message
	processing  map[string]bool
	activeConvs map[string]*ActiveConv
}

// New creates a Router. flowise may be nil — if nil, natural chat falls back
// to ai-vision-service's Gemini endpoint. searchBaseURL is the base URL of
// product-search-service (e.g. "http://product-search-service:8101").
func New(wa *whatsmeow.Client, store *state.Store, cache *state.CustomerCache, ai *client.AIVisionClient, flowise *client.FlowiseClient, ownerFlowise *client.FlowiseClient, ownerPhones map[string]bool, search *client.ProductSearchClient, searchBaseURL string) *Router {
	return &Router{
		wa:               wa,
		store:            store,
		cache:            cache,
		ai:               ai,
		flowise:          flowise,
		ownerFlowise:     ownerFlowise,
		ownerPhones:      ownerPhones,
		search:           search,
		catalogExportURL: searchBaseURL + "/export/csv",
		notif:            notification.NewNotifier(wa),
		marketingNumbers: notification.GetAllMarketingNumbers(),
		lastMsgTime:      make(map[string]int64),
		msgCount:         make(map[string]int),
		queues:           make(map[string][]*events.Message),
		processing:       make(map[string]bool),
		activeConvs:      make(map[string]*ActiveConv),
	}
}

// generateNatural calls Flowise if configured, falling back to Gemini via
// ai-vision-service. Returns "" if both are unavailable (callers use
// hardcoded fallback responses).
func (r *Router) generateNatural(ctx context.Context, message, phone, customerName string, history []string, isGreeting, isFirstTime bool) string {
	if r.flowise != nil {
		if resp := r.flowise.GenerateNatural(ctx, message, phone, customerName, history, isGreeting, isFirstTime); resp != "" {
			return resp
		}
	}
	return r.ai.GenerateNatural(ctx, message, phone, customerName, history, isGreeting, isFirstTime)
}

// HandleEvent is the whatsmeow event handler. It filters for incoming text /
// media messages and feeds them into the per-phone throttle+queue.
func (r *Router) HandleEvent(rawEvt interface{}) {
	evt, ok := rawEvt.(*events.Message)
	if !ok || evt.Info.IsFromMe {
		return
	}
	r.processMessage(evt)
}

// processMessage ports messageHandler.processMessage (line 79-150).
func (r *Router) processMessage(evt *events.Message) {
	phone := evt.Info.Sender.User

	if r.isMarketingNumber(phone) {
		log.Printf("skipping marketing number: %s", phone)
		return
	}

	_ = r.store.Set(phone, "lastMessageIsFromCustomer", true)

	r.mu.Lock()

	now := time.Now().UnixMilli()
	if r.lastMsgTime[phone] == 0 {
		r.lastMsgTime[phone] = now
		r.msgCount[phone] = 1
	} else {
		elapsed := now - r.lastMsgTime[phone]
		if elapsed < throttleWindow.Milliseconds() {
			r.msgCount[phone]++
			if r.msgCount[phone] > maxMsgPerWindow {
				count := r.msgCount[phone]
				r.mu.Unlock()
				if count == maxMsgPerWindow+1 {
					msg := "Mohon ditunggu sebentar ya, Bobi sedang memproses pesan Anda 🤖\n\nUntuk pengalaman terbaik, silakan kirim satu pesan dan tunggu respons saya ya."
					r.reply(context.Background(), evt, msg)
					_ = r.store.AddToHistory(phone, "assistant", msg)
				}
				return
			}
		} else {
			r.lastMsgTime[phone] = now
			r.msgCount[phone] = 1
		}
	}

	if len(r.queues[phone]) >= maxQueueSize {
		r.mu.Unlock()
		msg := "Mohon maaf, Bobi sedang memproses beberapa pesan sebelumnya. Silakan tunggu sebentar ya 🙏"
		r.reply(context.Background(), evt, msg)
		_ = r.store.AddToHistory(phone, "assistant", msg)
		return
	}
	r.queues[phone] = append(r.queues[phone], evt)

	if r.processing[phone] {
		r.mu.Unlock()
		return
	}
	r.processing[phone] = true
	r.mu.Unlock()

	go r.processMessageQueue(phone)
}

// processMessageQueue ports messageHandler.processMessageQueue (line 153-181).
func (r *Router) processMessageQueue(phone string) {
	defer func() {
		r.mu.Lock()
		r.processing[phone] = false
		r.mu.Unlock()
	}()

	for {
		r.mu.Lock()
		if len(r.queues[phone]) == 0 {
			r.mu.Unlock()
			return
		}
		evt := r.queues[phone][0]
		r.queues[phone] = r.queues[phone][1:]
		hasMore := len(r.queues[phone]) > 0
		r.mu.Unlock()

		if err := r.handleSingleMessage(evt); err != nil {
			log.Printf("handleSingleMessage error for %s: %v", phone, err)
		}

		if hasMore {
			time.Sleep(interMsgDelay)
		}
	}
}

// reply sends a plain text message to the same chat the event came from.
func (r *Router) reply(ctx context.Context, evt *events.Message, text string) {
	msg := &waE2E.Message{Conversation: proto.String(text)}
	if _, err := r.wa.SendMessage(ctx, evt.Info.Chat, msg); err != nil {
		log.Printf("SendMessage to %s error: %v", evt.Info.Chat, err)
	}
}

// replyDocument uploads data as a file attachment and sends it to the chat.
func (r *Router) replyDocument(ctx context.Context, evt *events.Message, data []byte, filename, mimetype string) error {
	uploaded, err := r.wa.Upload(ctx, data, whatsmeow.MediaDocument)
	if err != nil {
		return fmt.Errorf("upload document: %w", err)
	}
	msg := &waE2E.Message{
		DocumentMessage: &waE2E.DocumentMessage{
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(data))),
			Mimetype:      proto.String(mimetype),
			FileName:      proto.String(filename),
		},
	}
	if _, err := r.wa.SendMessage(ctx, evt.Info.Chat, msg); err != nil {
		return fmt.Errorf("send document: %w", err)
	}
	return nil
}

// fetchURL downloads bytes from a URL using a 60s timeout.
func fetchURL(ctx context.Context, url string) ([]byte, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// isMarketingNumber checks if phone is in the marketing numbers list.
// Populated in 3c; returns false for all numbers in 3b.
func (r *Router) isMarketingNumber(phone string) bool {
	for _, n := range r.marketingNumbers {
		if n == phone {
			return true
		}
	}
	return false
}

// setActiveConv sets (or replaces) the active conversation for phone.
func (r *Router) setActiveConv(phone string, ac *ActiveConv) {
	r.mu.Lock()
	r.activeConvs[phone] = ac
	r.mu.Unlock()
}

// deleteActiveConv removes the active conversation for phone.
func (r *Router) deleteActiveConv(phone string) {
	r.mu.Lock()
	delete(r.activeConvs, phone)
	r.mu.Unlock()
}

// msgBody extracts the text body from a whatsmeow message event.
func msgBody(evt *events.Message) string {
	if text := evt.Message.GetConversation(); text != "" {
		return text
	}
	if ext := evt.Message.GetExtendedTextMessage(); ext != nil {
		return ext.GetText()
	}
	return ""
}

// hasMedia returns true if the event carries a media attachment.
func hasMedia(evt *events.Message) bool {
	msg := evt.Message
	return msg.GetImageMessage() != nil ||
		msg.GetVideoMessage() != nil ||
		msg.GetDocumentMessage() != nil ||
		msg.GetAudioMessage() != nil ||
		msg.GetStickerMessage() != nil
}

// stubReply sends a "feature in development" placeholder for handlers not yet
// ported (3c-3h).
func (r *Router) stubReply(ctx context.Context, evt *events.Message, featureName string) {
	phone := evt.Info.Sender.User
	msg := fmt.Sprintf("Fitur %s sedang dalam pengembangan. Ketik /help untuk bantuan atau hubungi marketing kami.", featureName)
	r.reply(ctx, evt, msg)
	_ = r.store.AddToHistory(phone, "assistant", msg)
}

// nowMs returns the current time in milliseconds.
func nowMs() int64 {
	return time.Now().UnixMilli()
}
