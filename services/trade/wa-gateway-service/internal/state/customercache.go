package state

import (
	"context"
	"strings"
	"time"

	"wa-gateway-service/internal/client"
)

// customerCacheTTL is the 30-second memory-cache validity window from
// getCustomerInfoEnhanced (line ~2304).
const customerCacheTTL = 30 * time.Second

// CachedCustomer is the shape stored under the "customer" key, mirroring the
// "enhancedCustomer"/"vipOnlyCustomer" objects built by
// getCustomerInfoEnhanced (line 2301-2434). The lookup source is carried by
// the embedded Customer.Source field (set to "phone_lookup"/"company_lookup"
// below, or already "vip_only" for the VIP-only fallback).
type CachedCustomer struct {
	client.Customer
	CacheSource string `json:"cacheSource,omitempty"`
	LastCached  int64  `json:"lastCached,omitempty"`
}

// CustomerCache wraps client.CustomerPricingClient with the 30s customer-info
// cache and negative cache from messageHandler's getCustomerInfoEnhanced.
type CustomerCache struct {
	store   *Store
	pricing *client.CustomerPricingClient
}

func NewCustomerCache(store *Store, pricing *client.CustomerPricingClient) *CustomerCache {
	return &CustomerCache{store: store, pricing: pricing}
}

// GetCustomerInfoEnhanced ports messageHandler.getCustomerInfoEnhanced
// (line 2301-2434).
//
// Lookup order (Node): (1) phone lookup, (2) company-name lookup (only if
// userData.company is set), (3) VIP-only fallback on the user's own phone.
// customer-pricing-service's GET /customer?phone= already combines (1) and
// (3) (phone lookup with an immediate VIP-only fallback). To preserve Node's
// precedence exactly, a vip_only result from that call is only accepted
// after confirming step (2) doesn't also match.
//
// The negative-cache READ branch in Node is `if (false && ...)` - i.e. dead
// code - so it is not ported here, but the negative-cache WRITES
// (customerNotFound/customerNotFoundTime on "not found", cleared via
// clearNegativeCache on success) are still ported for fidelity.
func (cc *CustomerCache) GetCustomerInfoEnhanced(ctx context.Context, phone string) (*CachedCustomer, error) {
	var cached CachedCustomer
	found, err := cc.store.Get(phone, "customer", &cached)
	if err != nil {
		return nil, err
	}
	if found && cached.Nama != "" {
		var cacheTime int64
		if _, err := cc.store.Get(phone, "customerCacheTime", &cacheTime); err != nil {
			return nil, err
		}
		if time.Since(time.UnixMilli(cacheTime)) < customerCacheTTL {
			result := cached
			result.CacheSource = "memory"
			result.LastCached = cacheTime
			return &result, nil
		}
	}

	customer, err := cc.pricing.GetCustomer(ctx, phone)
	if err != nil {
		return nil, err
	}

	// Step (1): phone lookup matched (not the vip_only synthetic fallback).
	if customer != nil && customer.Source != "vip_only" {
		vip, err := cc.pricing.GetCustomerVip(ctx, phone)
		if err != nil {
			return nil, err
		}

		enhanced := CachedCustomer{Customer: *customer, CacheSource: "database"}
		enhanced.Source = "phone_lookup"
		enhanced.IsVip = vip != nil
		enhanced.VipInfo = vip

		if err := cc.cacheCustomer(phone, &enhanced); err != nil {
			return nil, err
		}
		return &enhanced, nil
	}

	// Step (2): company-name lookup, takes precedence over the vip_only
	// fallback from step (3).
	var userCompany string
	if _, err := cc.store.Get(phone, "company", &userCompany); err != nil {
		return nil, err
	}
	if companyName := strings.TrimSpace(userCompany); companyName != "" {
		byCompany, err := cc.pricing.GetCustomerByCompany(ctx, companyName)
		if err != nil {
			return nil, err
		}
		if byCompany != nil && byCompany.Nama != "" && byCompany.Nomor != "" && byCompany.Nomor != "-" {
			// Re-fetch via /customer?phone= using the matched customer's own
			// number, so the same Nama/Pulau defaulting that
			// GetCustomerDetailWithIsland applies for phone/vip_only lookups
			// also applies here.
			islanded, err := cc.pricing.GetCustomer(ctx, byCompany.Nomor)
			if err != nil {
				return nil, err
			}
			if islanded == nil {
				islanded = byCompany
			}

			vip, err := cc.pricing.GetCustomerVip(ctx, byCompany.Nomor)
			if err != nil {
				return nil, err
			}

			enhanced := CachedCustomer{Customer: *islanded, CacheSource: "database"}
			enhanced.Source = "company_lookup"
			enhanced.IsVip = vip != nil
			enhanced.VipInfo = vip

			if err := cc.cacheCustomer(phone, &enhanced); err != nil {
				return nil, err
			}
			return &enhanced, nil
		}
	}

	// Step (3): VIP-only fallback on the user's own phone, already resolved
	// by GET /customer?phone= above.
	if customer != nil && customer.Source == "vip_only" {
		enhanced := CachedCustomer{Customer: *customer, CacheSource: "vip_database"}
		if err := cc.cacheCustomer(phone, &enhanced); err != nil {
			return nil, err
		}
		return &enhanced, nil
	}

	if err := cc.store.Set(phone, "isRegistered", false); err != nil {
		return nil, err
	}
	if err := cc.store.Set(phone, "customerNotFound", true); err != nil {
		return nil, err
	}
	if err := cc.store.Set(phone, "customerNotFoundTime", time.Now().UnixMilli()); err != nil {
		return nil, err
	}
	return nil, nil
}

func (cc *CustomerCache) cacheCustomer(phone string, c *CachedCustomer) error {
	if err := cc.store.Set(phone, "customer", c); err != nil {
		return err
	}
	if err := cc.store.Set(phone, "customerCacheTime", time.Now().UnixMilli()); err != nil {
		return err
	}
	if err := cc.store.Set(phone, "isRegistered", true); err != nil {
		return err
	}
	return cc.clearNegativeCache(phone)
}

// clearNegativeCache ports messageHandler.clearNegativeCache (line 2431).
func (cc *CustomerCache) clearNegativeCache(phone string) error {
	if err := cc.store.Set(phone, "customerNotFound", nil); err != nil {
		return err
	}
	return cc.store.Set(phone, "customerNotFoundTime", nil)
}

// IsRegisteredCustomer ports messageHandler.isRegisteredCustomer (line 2441).
func (cc *CustomerCache) IsRegisteredCustomer(ctx context.Context, phone string) (bool, error) {
	customer, err := cc.GetCustomerInfoEnhanced(ctx, phone)
	if err != nil {
		return false, err
	}
	return customer != nil && customer.Nama != "" && customer.ID != nil, nil
}

// GetCustomerDetailWithIsland ports messageHandler.getCustomerDetailWithIsland
// (line 2456). The Nama/Pulau defaulting it performs on top of
// getCustomerInfoEnhanced is already applied for every lookup path above (via
// customer-pricing-service's GetCustomerDetailWithIsland), so this is an
// alias.
func (cc *CustomerCache) GetCustomerDetailWithIsland(ctx context.Context, phone string) (*CachedCustomer, error) {
	return cc.GetCustomerInfoEnhanced(ctx, phone)
}

// GetCustomerInfo ports messageHandler.getCustomerInfo (line 2558), a
// backward-compatibility alias for getCustomerInfoEnhanced.
func (cc *CustomerCache) GetCustomerInfo(ctx context.Context, phone string) (*CachedCustomer, error) {
	return cc.GetCustomerInfoEnhanced(ctx, phone)
}

// GetCustomerByCompany looks up a customer by company name via
// customer-pricing-service. Returns nil, nil when not found (404).
// Ports the customerService.verifyCustomerByCompany call in completeRegistration.
func (cc *CustomerCache) GetCustomerByCompany(ctx context.Context, name string) (*client.Customer, error) {
	return cc.pricing.GetCustomerByCompany(ctx, name)
}

// GetCustomerPrice ports messageHandler.getCustomerPrice (line 2495):
// fully delegated to customer-pricing-service's POST /price, which already
// implements the VIP check, registration check, island pricing factor, and
// base price selection.
func (cc *CustomerCache) GetCustomerPrice(ctx context.Context, product client.Product, phone string) (float64, error) {
	resp, err := cc.pricing.GetPrice(ctx, client.PriceRequest{
		PhoneNumber: phone,
		HargaNum: client.PriceHargaNum{
			Customer:    product.HargaNum.Customer,
			NonCustomer: product.HargaNum.NonCustomer,
		},
	})
	if err != nil {
		return 0, err
	}
	return resp.Price, nil
}
