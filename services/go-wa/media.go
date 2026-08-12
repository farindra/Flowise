package main

import (
	"context"
	"fmt"
	"strconv"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

// defaultMaxMediaBytes caps a single outbound image. Overridable via
// MAX_MEDIA_BYTES so a deployment can raise it without a rebuild.
const defaultMaxMediaBytes = 5 << 20 // 5 MB

var maxMediaBytes = defaultMaxMediaBytes

func initMediaLimits() {
	if v := envOr("MAX_MEDIA_BYTES", ""); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxMediaBytes = n
		}
	}
}

// allowedImageMimes is the set of image types WhatsApp renders reliably.
var allowedImageMimes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
}

// SendImage uploads data to WhatsApp's media servers and sends it as an image
// message, returning the WhatsApp message ID. An empty caption sends the image
// on its own.
func (s *WASession) SendImage(ctx context.Context, phone string, data []byte, mime, caption string) (string, error) {
	jid, err := parseJID(normalizeWAPhone(phone))
	if err != nil {
		return "", fmt.Errorf("invalid phone: %w", err)
	}

	up, err := s.waClient.Upload(ctx, data, whatsmeow.MediaImage)
	if err != nil {
		return "", fmt.Errorf("upload: %w", err)
	}

	img := &waE2E.ImageMessage{
		Mimetype:      proto.String(mime),
		URL:           &up.URL,
		DirectPath:    &up.DirectPath,
		MediaKey:      up.MediaKey,
		FileEncSHA256: up.FileEncSHA256,
		FileSHA256:    up.FileSHA256,
		FileLength:    &up.FileLength,
	}
	if caption != "" {
		img.Caption = proto.String(mdToWA(caption))
	}

	resp, err := s.waClient.SendMessage(ctx, jid, &waE2E.Message{ImageMessage: img})
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}
