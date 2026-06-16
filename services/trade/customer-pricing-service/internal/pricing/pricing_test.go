package pricing

import "testing"

func TestFormatPhoneNumber(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"081234567890", "81234567890"},        // 08xx -> 62 -> strip 62
		{"6281234567890", "81234567890"},       // already 62xx -> strip 62
		{"81234567890", "81234567890"},         // bare local number, no 0/62 prefix
		{"021234567", "21234567"},              // landline 0xx -> strip leading 0 only
		{"+62-812-3456-7890", "81234567890"},   // punctuation stripped first
		{"0812345678/0813456789", "812345678"}, // multiple numbers - take first
		{"123", ""},                            // too short
	}

	for _, c := range cases {
		got := FormatPhoneNumber(c.in)
		if got != c.want {
			t.Errorf("FormatPhoneNumber(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestPhoneNumbersMatch(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"081234567890", "6281234567890", true},
		{"81234567890", "081234567890", true},
		{"6281234567890", "81234567890", true},
		{"081234567890", "081234567891", false},
		{"", "081234567890", false},
	}

	for _, c := range cases {
		got := PhoneNumbersMatch(c.a, c.b)
		if got != c.want {
			t.Errorf("PhoneNumbersMatch(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestGetIslandFromProvince(t *testing.T) {
	cases := []struct {
		province string
		want     string
	}{
		{"Jawa Timur", "Pulau Jawa"},
		{"jawa timur", "Pulau Jawa"},
		{"Bandar Lampung", "Pulau Sumatra"},
		{"Maluku Utara", "Pulau Maluku"},
		{"", "Pulau Kalimantan"},
		{"Kota Pontianak", "Pulau Kalimantan"}, // substring match
		{"Antartika", "Pulau Kalimantan"},      // no match -> default
	}

	for _, c := range cases {
		got := GetIslandFromProvince(c.province)
		if got != c.want {
			t.Errorf("GetIslandFromProvince(%q) = %q, want %q", c.province, got, c.want)
		}
	}
}

func id(v int64) *int64 { return &v }

func newTestStore() *Store {
	s := &Store{}
	s.customers = []Customer{
		{ID: id(1), Nomor: "6281234567890", Nama: "Toko Jaya", Pulau: "Pulau Jawa", BillingAddressProvinsi: "Jawa Timur"},
		{ID: id(2), Nomor: "082233445566", Nama: "Sumatra Bearing", Pulau: "Sumatera", BillingAddressProvinsi: "Bandar Lampung"},
		{ID: id(3), Nomor: "-", Nama: "No Phone Customer"},
	}
	return s
}

func TestFindCustomerByPhoneSimple(t *testing.T) {
	s := newTestStore()

	// exact match after normalization
	c := s.VerifyCustomer("6281234567890")
	if c == nil || c.Nama != "Toko Jaya" {
		t.Fatalf("expected Toko Jaya, got %+v", c)
	}

	// suffix match: different prefix, same last 8 digits
	c2 := s.VerifyCustomer("6282233445566")
	if c2 == nil || c2.Nama != "Sumatra Bearing" {
		t.Fatalf("expected Sumatra Bearing via suffix match, got %+v", c2)
	}

	// not found
	c3 := s.VerifyCustomer("6289999999999")
	if c3 != nil {
		t.Fatalf("expected nil, got %+v", c3)
	}
}

func TestGetCustomerDetailWithIsland_SelfCorrectsInvalidPulau(t *testing.T) {
	s := newTestStore()
	vipStore := &VipStore{}

	// customer 2 has pulau="Sumatera" (not a valid pricingRules key) and
	// billing_address_provinsi="Bandar Lampung" -> should resolve to
	// "Pulau Sumatra" via province fallback.
	c := s.GetCustomerDetailWithIsland("6282233445566", vipStore)
	if c == nil {
		t.Fatal("expected customer, got nil")
	}
	if c.Pulau != "Pulau Sumatra" {
		t.Errorf("Pulau = %q, want %q", c.Pulau, "Pulau Sumatra")
	}
}

func TestGetCustomerDetailWithIsland_VipOnlyFallback(t *testing.T) {
	s := newTestStore()
	vipStore := &VipStore{}
	vipStore.customers = []VipCustomer{
		{Nama: "VIP Only Co", Wilayah: "Jakarta", DiscountPercentage: 5, PhoneNumbers: []string{"6289988877766"}},
	}

	c := s.GetCustomerDetailWithIsland("6289988877766", vipStore)
	if c == nil {
		t.Fatal("expected vip_only customer, got nil")
	}
	if c.Nama != "VIP Only Co" || !c.IsVip || c.Source != "vip_only" {
		t.Errorf("unexpected vip_only customer: %+v", c)
	}
	if c.Pulau != "Pulau Kalimantan" {
		t.Errorf("Pulau = %q, want default Pulau Kalimantan", c.Pulau)
	}
	if c.ID != nil {
		t.Errorf("expected nil ID for vip_only customer, got %v", *c.ID)
	}
}

func TestFindVipCustomer(t *testing.T) {
	vipStore := &VipStore{}
	vipStore.customers = []VipCustomer{
		{Nama: "Jaya Utama Bearing", DiscountPercentage: 3, PhoneNumbers: []string{"628123548833"}},
	}

	// exact match
	if v := vipStore.FindVipCustomer("628123548833"); v == nil {
		t.Fatal("expected exact match")
	}

	// suffix match: last-10-digit overlap with different prefix/length
	if v := vipStore.FindVipCustomer("08123548833"); v == nil {
		t.Fatal("expected suffix match")
	}

	if v := vipStore.FindVipCustomer("6280000000000"); v != nil {
		t.Fatalf("expected nil, got %+v", v)
	}
}

func TestCalculateVipPrice(t *testing.T) {
	vipStore := &VipStore{}
	vipStore.customers = []VipCustomer{
		{Nama: "Jaya Utama Bearing", DiscountPercentage: 3, PhoneNumbers: []string{"628123548833"}},
	}

	result := vipStore.CalculateVipPrice(100000, "628123548833")
	if !result.IsVip {
		t.Fatal("expected isVip=true")
	}
	if result.Price != 97000 {
		t.Errorf("price = %v, want 97000", result.Price)
	}

	// hargaCustomer == 0 -> no customer price -> not VIP
	result2 := vipStore.CalculateVipPrice(0, "628123548833")
	if result2.IsVip {
		t.Fatal("expected isVip=false when hargaCustomer is 0")
	}

	// not a VIP phone
	result3 := vipStore.CalculateVipPrice(100000, "6289999999999")
	if result3.IsVip {
		t.Fatal("expected isVip=false for non-vip phone")
	}
}

func TestGetCustomerPrice_RegisteredJawaCustomer(t *testing.T) {
	s := newTestStore()
	vipStore := &VipStore{}

	req := PriceRequest{
		PhoneNumber: "6281234567890",
		HargaNum:    HargaNum{Customer: 100000, NonCustomer: 110000},
	}

	resp := GetCustomerPrice(s, vipStore, req)
	if resp.IsVip {
		t.Fatal("expected isVip=false")
	}
	if !resp.IsRegistered {
		t.Fatal("expected isRegistered=true")
	}
	if resp.Pulau != "Pulau Jawa" {
		t.Errorf("pulau = %q, want Pulau Jawa", resp.Pulau)
	}
	// 100000 * 1.01 = 101000
	if resp.Price != 101000 {
		t.Errorf("price = %v, want 101000", resp.Price)
	}
}

func TestGetCustomerPrice_NonCustomer(t *testing.T) {
	s := newTestStore()
	vipStore := &VipStore{}

	req := PriceRequest{
		PhoneNumber: "6289999999999",
		HargaNum:    HargaNum{Customer: 100000, NonCustomer: 110000},
	}

	resp := GetCustomerPrice(s, vipStore, req)
	if resp.IsRegistered {
		t.Fatal("expected isRegistered=false")
	}
	if resp.Pulau != "nonCustomer" {
		t.Errorf("pulau = %q, want nonCustomer", resp.Pulau)
	}
	// round(110000 * 1.08182) = round(119000.2) = 119000
	if resp.Price != 119000 {
		t.Errorf("price = %v, want 119000", resp.Price)
	}
}

func TestGetCustomerPrice_VipTakesPriority(t *testing.T) {
	s := newTestStore()
	vipStore := &VipStore{}
	// customer 1 (Toko Jaya, 6281234567890) is ALSO VIP with 10% discount
	vipStore.customers = []VipCustomer{
		{Nama: "Toko Jaya VIP", DiscountPercentage: 10, PhoneNumbers: []string{"6281234567890"}},
	}

	req := PriceRequest{
		PhoneNumber: "6281234567890",
		HargaNum:    HargaNum{Customer: 100000, NonCustomer: 110000},
	}

	resp := GetCustomerPrice(s, vipStore, req)
	if !resp.IsVip {
		t.Fatal("expected isVip=true to take priority over registered pricing")
	}
	// round(100000 * (1 - 0.10)) = 90000
	if resp.Price != 90000 {
		t.Errorf("price = %v, want 90000", resp.Price)
	}
	if resp.Discount != 10 {
		t.Errorf("discount = %v, want 10", resp.Discount)
	}
}
