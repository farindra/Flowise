package vision

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"ai-vision-service/internal/gemini"
	"ai-vision-service/internal/ratelimit"
)

const (
	chatRateLimitMax = 20
	chatTimeout      = 30 * time.Second
)

// Chatter handles AI-generated natural conversation responses, porting
// aiService.generateNaturalGreeting() (line 818) and
// aiService.generateNaturalResponse() (line 923).
type Chatter struct {
	gemini  *gemini.Client
	limiter *ratelimit.Limiter
}

func NewChatter(g *gemini.Client) *Chatter {
	return &Chatter{
		gemini:  g,
		limiter: ratelimit.NewLimiter(),
	}
}

// NaturalChatRequest is the JSON body for POST /generate-natural.
type NaturalChatRequest struct {
	Message     string   `json:"message"`
	PhoneNumber string   `json:"phoneNumber"`
	CustomerName string  `json:"customerName,omitempty"`
	History     []string `json:"history,omitempty"` // recent assistant+user lines
	IsGreeting  bool     `json:"isGreeting"`
	IsFirstTime bool     `json:"isFirstTime"`
}

// NaturalChatResponse is the JSON response from POST /generate-natural.
type NaturalChatResponse struct {
	Response string `json:"response"`
}

// GenerateNatural dispatches to greeting or response generation based on IsGreeting.
func (c *Chatter) GenerateNatural(ctx context.Context, req *NaturalChatRequest) string {
	phone := req.PhoneNumber
	if phone == "" {
		phone = "unknown"
	}
	if req.IsGreeting {
		key := fmt.Sprintf("greeting_%s", phone)
		if c.limiter.IsRateLimited(key, 10) {
			return c.fallbackGreeting(req.CustomerName, req.IsFirstTime)
		}
		c.limiter.AddCall(key)
		result, err := c.callGemini(ctx, c.greetingPrompt(req), chatTimeout)
		if err != nil || result == "" {
			return c.fallbackGreeting(req.CustomerName, req.IsFirstTime)
		}
		return result
	}

	key := fmt.Sprintf("chat_%s", phone)
	if c.limiter.IsRateLimited(key, chatRateLimitMax) {
		return "Wah, sepertinya lagi ramai banget nih. Coba chat lagi sebentar ya 😅"
	}
	c.limiter.AddCall(key)
	result, err := c.callGemini(ctx, c.responsePrompt(req), chatTimeout)
	if err != nil || result == "" {
		return c.fallbackResponse(req.CustomerName)
	}
	return result
}

func (c *Chatter) callGemini(ctx context.Context, prompt string, timeout time.Duration) (string, error) {
	tctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return c.gemini.GenerateContent(tctx, []gemini.Part{{Text: prompt}})
}

func (c *Chatter) greetingPrompt(req *NaturalChatRequest) string {
	hour := time.Now().Hour()
	timeOfDay := "pagi"
	if hour >= 11 && hour < 15 {
		timeOfDay = "siang"
	} else if hour >= 15 && hour < 19 {
		timeOfDay = "sore"
	} else if hour >= 19 || hour < 5 {
		timeOfDay = "malam"
	}
	customer := req.CustomerName
	if customer == "" {
		customer = "Customer baru"
	}
	return fmt.Sprintf(`Anda adalah Bobi, asisten customer service Ocean Bearing yang sangat ramah dan natural seperti manusia.

KARAKTER ANDA:
- Sangat ramah dan hangat seperti teman
- Natural dalam berbicara, tidak kaku atau robotic
- Bisa basa-basi ringan sebelum ke topik bisnis
- Menggunakan bahasa sehari-hari yang santai tapi tetap sopan
- Responsif terhadap mood customer
- PENTING: JANGAN PERNAH menggunakan tanda seru (!) dalam setiap balasan Anda. Gunakan tanda titik atau bahasa yang lebih sopan.
- PENTING: JANGAN PERNAH menggunakan sebutan "Kak" atau "Kakak". Gunakan bahasa yang lebih general dan profesional.

SITUASI SAAT INI:
- Waktu: %s
- Customer: %s
- Pertama kali hari ini: %s
- Pesan customer: "%s"

TUGAS ANDA:
1. Balas salam dengan natural dan hangat
2. Bisa sedikit basa-basi atau small talk yang relevan
3. Jangan langsung to-the-point tentang produk
4. Buat customer merasa nyaman dan welcome
5. Akhiri dengan pertanyaan terbuka yang mengundang conversation

GAYA BICARA:
- Gunakan emoji yang tepat (tapi jangan berlebihan)
- Bahasa Indonesia yang natural dan friendly
- Bisa pakai kata-kata seperti "gimana", "nih", "dong", dll
- Sesuaikan dengan tone customer

JANGAN:
- Langsung kasih instruksi panjang tentang cara cari produk
- Terlalu formal atau kaku
- Copy-paste template response
- Bahas harga atau teknis dulu

Berikan respons yang natural dan hangat:`, timeOfDay, customer, formatBool(req.IsFirstTime), req.Message)
}

func (c *Chatter) responsePrompt(req *NaturalChatRequest) string {
	customer := req.CustomerName
	if customer == "" {
		customer = "Customer"
	}
	historyStr := strings.Join(req.History, "\n")
	return fmt.Sprintf(`Anda adalah Bobi, asisten customer service Ocean Bearing yang sangat natural dan friendly.

KARAKTER ANDA:
- Bicara seperti manusia biasa, tidak kaku
- Ramah, santai, tapi tetap profesional
- Bisa basa-basi dan small talk
- Responsif terhadap mood customer
- Paham konteks percakapan
- Seperti teman yang kebetulan kerja di toko bearing

KEMAMPUAN ANDA:
- Membantu cari produk bearing dan otomotif
- Kasih info umum tentang produk
- Sambungin ke tim marketing kalau perlu
- Bisa ngobrol santai sebelum ke topik bisnis

GAYA BICARA:
- Natural dan conversational
- Pakai bahasa sehari-hari yang friendly
- Emoji yang tepat (jangan berlebihan)
- Sesuaikan dengan tone customer
- Bisa pakai "gimana", "nih", "dong", "sih", dll

CUSTOMER: %s
PESAN TERAKHIR: "%s"

RIWAYAT SINGKAT:
%s

Berikan respons yang natural dan sesuai konteks:`, customer, req.Message, historyStr)
}

func (c *Chatter) fallbackGreeting(customerName string, isFirstTime bool) string {
	hour := time.Now().Hour()
	greeting := "Halo"
	if hour >= 5 && hour < 11 {
		greeting = "Selamat pagi"
	} else if hour >= 11 && hour < 15 {
		greeting = "Selamat siang"
	} else if hour >= 15 && hour < 19 {
		greeting = "Selamat sore"
	} else {
		greeting = "Selamat malam"
	}
	responses := []string{
		fmt.Sprintf("%s 😊 Gimana kabarnya hari ini? Saya Bobi dari Ocean Bearing nih. Ada yang bisa dibantu?", greeting),
		fmt.Sprintf("%s 👋 Senang banget ada yang mampir. Saya Bobi, siap bantu-bantu. Gimana, lagi cari apa nih?", greeting),
		fmt.Sprintf("%s. Hehe, saya Bobi dari Ocean Bearing. Semoga harinya lancar ya. Ada yang perlu bantuan?", greeting),
		fmt.Sprintf("%s 😄 Salam kenal, saya Bobi. Lagi santai atau ada yang mau dicari nih?", greeting),
	}
	return responses[rand.Intn(len(responses))]
}

func (c *Chatter) fallbackResponse(customerName string) string {
	responses := []string{
		"Hmm, saya kurang ngerti nih. Bisa cerita lebih jelas atau langsung ketik nama produk yang dicari? 😊",
		"Mohon maaf, saya belum paham maksudnya. Mau cari bearing apa, atau ada yang bisa saya bantu?",
		"Hehe, saya agak bingung nih. Coba ketik nama atau kode bearing yang dibutuhkan ya.",
	}
	return responses[rand.Intn(len(responses))]
}

func formatBool(b bool) string {
	if b {
		return "Ya"
	}
	return "Tidak"
}
