// Package waclient is a thin wrapper around whatsmeow used for the Phase 1
// spike: pairing via QR code, persisting the session to sqlite, and replying
// to incoming text messages with a simple echo.
package waclient

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
	"rsc.io/qr"

	_ "modernc.org/sqlite"
)

// Client wraps a whatsmeow client backed by a sqlite-based device store.
type Client struct {
	WA              *whatsmeow.Client
	qrPath          string
	qrReady         chan struct{}
	externalHandler func(interface{})
}

// SetEventHandler replaces the default echo handler with fn. Call this once
// during startup to wire the router's HandleEvent into the whatsmeow event
// pipeline. Thread-safe: the assignment is not concurrent with incoming events
// because it is called before Connect().
func (c *Client) SetEventHandler(fn func(interface{})) {
	c.externalHandler = fn
}

// QRPath returns the filesystem path where the latest pairing QR code (PNG)
// is written, if any. The file only exists while pairing is pending.
func (c *Client) QRPath() string {
	return c.qrPath
}

// PairPhone requests a pairing code for linking via phone number instead of
// scanning a QR code. It blocks until the underlying connection has emitted
// its first QR code event (required by whatsmeow before requesting a code).
func (c *Client) PairPhone(ctx context.Context, phone string) (string, error) {
	select {
	case <-c.qrReady:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	return c.WA.PairPhone(ctx, phone, true, whatsmeow.PairClientChrome, "Chrome (Linux)")
}

// New opens (or creates) the sqlite device store under dataDir and prepares
// a whatsmeow client for the first (or newly created) device.
func New(ctx context.Context, dataDir string) (*Client, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	dbPath := filepath.Join(dataDir, "whatsmeow.db")
	dbLog := waLog.Stdout("Database", "INFO", true)
	container, err := sqlstore.New(ctx, "sqlite", "file:"+dbPath+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(10000)", dbLog)
	if err != nil {
		return nil, fmt.Errorf("open sqlstore: %w", err)
	}

	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		return nil, fmt.Errorf("get device: %w", err)
	}

	clientLog := waLog.Stdout("Client", "INFO", true)
	waClient := whatsmeow.NewClient(deviceStore, clientLog)

	c := &Client{WA: waClient, qrPath: filepath.Join(dataDir, "qr.png"), qrReady: make(chan struct{})}
	waClient.AddEventHandler(c.handleEvent)

	return c, nil
}

// Connect connects to WhatsApp. If the device is not yet paired, it prints a
// QR code to stdout (scan it from WhatsApp -> Linked Devices) and blocks
// until pairing succeeds. On QR timeout it automatically disconnects and
// retries indefinitely so a new QR is always available without restarting.
func (c *Client) Connect(ctx context.Context) error {
	if c.WA.Store.ID != nil {
		return c.WA.Connect()
	}

	for {
		qrChan, err := c.WA.GetQRChannel(ctx)
		if err != nil {
			return fmt.Errorf("get qr channel: %w", err)
		}
		if err := c.WA.Connect(); err != nil {
			return fmt.Errorf("connect: %w", err)
		}

		timedOut := false
		var qrReadyClosed bool
		for evt := range qrChan {
			switch evt.Event {
			case whatsmeow.QRChannelEventCode:
				fmt.Println("=== Scan QR code ini dengan WhatsApp -> Linked Devices ===")
				qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
				if code, err := qr.Encode(evt.Code, qr.L); err == nil {
					if err := os.WriteFile(c.qrPath, code.PNG(), 0o644); err != nil {
						fmt.Println("gagal tulis QR PNG:", err)
					} else {
						fmt.Println("QR PNG ditulis ke", c.qrPath)
					}
				}
				if !qrReadyClosed {
					close(c.qrReady)
					qrReadyClosed = true
				}
			case "success":
				fmt.Println("=== Pairing sukses ===")
				_ = os.Remove(c.qrPath)
				return nil
			case "timeout":
				fmt.Println("=== QR code timeout, generate QR baru... ===")
				_ = os.Remove(c.qrPath)
				c.qrReady = make(chan struct{})
				qrReadyClosed = false
				timedOut = true
			default:
				if evt.Error != nil {
					fmt.Println("=== Pairing error:", evt.Error, "===")
				} else {
					fmt.Println("=== Login event:", evt.Event, "===")
				}
			}
		}

		if !timedOut {
			return nil
		}

		// Disconnect dulu sebelum minta QR baru
		c.WA.Disconnect()
		fmt.Println("=== Reconnect dalam 3 detik untuk QR baru... ===")
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
}

// Disconnect closes the WhatsApp connection.
func (c *Client) Disconnect() {
	c.WA.Disconnect()
}

// IsConnected returns true if the WA client is currently connected.
func (c *Client) IsConnected() bool {
	return c.WA.IsConnected()
}

// IsLoggedIn returns true if the WA client is logged in (has device ID).
func (c *Client) IsLoggedIn() bool {
	return c.WA.IsLoggedIn()
}

// PhoneNumber returns the connected phone number, or empty string.
func (c *Client) PhoneNumber() string {
	if c.WA.Store.ID == nil {
		return ""
	}
	return c.WA.Store.ID.User
}

// Logout disconnects and removes the device credentials, then reconnects to
// start the QR pairing flow automatically.
func (c *Client) Logout(ctx context.Context) error {
	if err := c.WA.Logout(ctx); err != nil {
		c.WA.Disconnect()
	}
	_ = os.Remove(c.qrPath)
	// Reset qrReady so PairPhone can be called after reconnect
	c.qrReady = make(chan struct{})
	// Reconnect in background to start QR flow
	go func() {
		if err := c.Connect(context.Background()); err != nil {
			fmt.Println("reconnect setelah logout error:", err)
		}
	}()
	return nil
}

// handleEvent dispatches incoming whatsmeow events. When an external handler
// has been registered via SetEventHandler, it is called instead of the Phase 1
// echo. This lets main.go wire the router without touching waclient internals.
func (c *Client) handleEvent(rawEvt interface{}) {
	// Intercept LoggedOut regardless of external handler — auto-restart QR flow
	if _, ok := rawEvt.(*events.LoggedOut); ok {
		fmt.Println("=== Device removed / LoggedOut — memulai QR flow baru dalam 3 detik... ===")
		_ = os.Remove(c.qrPath)
		c.qrReady = make(chan struct{})
		go func() {
			time.Sleep(3 * time.Second)
			if err := c.Connect(context.Background()); err != nil {
				fmt.Println("reconnect setelah LoggedOut error:", err)
			}
		}()
	}

	if c.externalHandler != nil {
		c.externalHandler(rawEvt)
		return
	}

	// Phase 1 echo fallback — only active before router is wired in.
	evt, ok := rawEvt.(*events.Message)
	if !ok || evt.Info.IsFromMe {
		return
	}

	text := evt.Message.GetConversation()
	if text == "" {
		if ext := evt.Message.GetExtendedTextMessage(); ext != nil {
			text = ext.GetText()
		}
	}
	if text == "" {
		return
	}

	reply := &waE2E.Message{
		Conversation: proto.String("echo: " + text),
	}
	if _, err := c.WA.SendMessage(context.Background(), evt.Info.Chat, reply); err != nil {
		fmt.Println("gagal kirim balasan:", err)
	} else {
		fmt.Printf("pesan dari %s diterima, dibalas: %q\n", evt.Info.Sender, text)
	}
}
