package shared

import "testing"

func TestAnalyzeInputContext_ProductCode(t *testing.T) {
	ctx := AnalyzeInputContext("6205", false, "idle")
	if ctx.Intent != "product_search" {
		t.Errorf("Intent = %q, want product_search", ctx.Intent)
	}
	if !ctx.HasProductCode {
		t.Error("HasProductCode = false, want true")
	}
	if ctx.Confidence < 0.8 {
		t.Errorf("Confidence = %v, want >= 0.8", ctx.Confidence)
	}
}

func TestAnalyzeInputContext_ProductKeyword(t *testing.T) {
	ctx := AnalyzeInputContext("cari bearing skf", false, "idle")
	if ctx.Intent != "product_search" {
		t.Errorf("Intent = %q, want product_search", ctx.Intent)
	}
	if !ctx.HasProductKeyword {
		t.Error("HasProductKeyword = false, want true")
	}
}

func TestAnalyzeInputContext_Greeting(t *testing.T) {
	ctx := AnalyzeInputContext("halo selamat pagi", false, "idle")
	if ctx.Intent != "greeting" {
		t.Errorf("Intent = %q, want greeting", ctx.Intent)
	}
	if !ctx.IsGreeting {
		t.Error("IsGreeting = false, want true")
	}
}

func TestAnalyzeInputContext_Question(t *testing.T) {
	// "apa kabar?" — "apa" triggers question, no product keywords.
	ctx := AnalyzeInputContext("apa kabar?", false, "idle")
	if ctx.Intent != "question" {
		t.Errorf("Intent = %q, want question", ctx.Intent)
	}
	if !ctx.IsQuestion {
		t.Error("IsQuestion = false, want true")
	}
}

func TestAnalyzeInputContext_RegistrationRequiredGeneral(t *testing.T) {
	ctx := AnalyzeInputContext("ok siap", false, "idle")
	if ctx.Intent != "registration_required" {
		t.Errorf("Intent = %q, want registration_required", ctx.Intent)
	}
}

func TestAnalyzeInputContext_Frustrated(t *testing.T) {
	ctx := AnalyzeInputContext("lanjut aja bro", false, "idle")
	if !ctx.IsFrustrated {
		t.Error("IsFrustrated = false, want true")
	}
}

func TestUpdateConversationContext_FrustrationAccumulates(t *testing.T) {
	ctx := ConversationContext{}
	input := InputContext{Intent: "product_search", IsFrustrated: true}
	updated, skipReg := UpdateConversationContext(ctx, input)

	if updated.FrustrationLevel != 1 {
		t.Errorf("FrustrationLevel = %d, want 1", updated.FrustrationLevel)
	}
	if skipReg {
		t.Error("skipReg = true after 1 frustration, want false (threshold is 2)")
	}
	if updated.LastIntent != "product_search" {
		t.Errorf("LastIntent = %q, want product_search", updated.LastIntent)
	}

	// Second frustrated message → level 2, skipReg = true
	updated2, skipReg2 := UpdateConversationContext(updated, input)
	if updated2.FrustrationLevel != 2 {
		t.Errorf("FrustrationLevel = %d, want 2", updated2.FrustrationLevel)
	}
	if !skipReg2 {
		t.Error("skipReg = false after 2 frustrations, want true")
	}
}

func TestUpdateConversationContext_FrustrationDecays(t *testing.T) {
	ctx := ConversationContext{FrustrationLevel: 3}
	input := InputContext{Intent: "product_search", IsFrustrated: false}
	updated, _ := UpdateConversationContext(ctx, input)
	if updated.FrustrationLevel != 2 {
		t.Errorf("FrustrationLevel = %d after decay, want 2", updated.FrustrationLevel)
	}
}

func TestUpdateConversationContext_IntentHistorySliced(t *testing.T) {
	ctx := ConversationContext{IntentHistory: []string{"a", "b", "c", "d", "e"}}
	input := InputContext{Intent: "greeting"}
	updated, _ := UpdateConversationContext(ctx, input)
	// slice(-4) keeps last 4 of existing, then appends new → 5 entries max
	if len(updated.IntentHistory) > 5 {
		t.Errorf("IntentHistory len = %d, want <= 5", len(updated.IntentHistory))
	}
	last := updated.IntentHistory[len(updated.IntentHistory)-1]
	if last != "greeting" {
		t.Errorf("last IntentHistory = %q, want greeting", last)
	}
}

func TestCalculateIntentConfidence(t *testing.T) {
	tests := []struct {
		msg    string
		intent string
		minC   float64
		maxC   float64
	}{
		{"6205", "product_search", 0.85, 0.95},
		{"cari bearing", "product_search", 0.75, 0.85},
		{"mau beli barang", "product_search", 0.55, 0.65},
		{"halo", "greeting", 0.85, 0.95},
		{"selamat pagi bos", "greeting", 0.65, 0.75},
		{"apa aja", "general", 0.45, 0.55},
	}
	for _, tt := range tests {
		got := CalculateIntentConfidence(tt.msg, tt.intent)
		if got < tt.minC || got > tt.maxC {
			t.Errorf("CalculateIntentConfidence(%q, %q) = %v, want [%v, %v]",
				tt.msg, tt.intent, got, tt.minC, tt.maxC)
		}
	}
}
