package main

import (
	"context"
	"strconv"
)

// flowiseWorkspaceID is the single workspace OB/SBB configuration lives in —
// matches the "variable" rows already used for TOKO_ALAMAT, CS_WA_1, etc.
const flowiseWorkspaceID = "614a13b1-d111-4b34-a740-a3712ce06412"

// unregisteredMarkupVarName is the Flowise Variable name staff edit directly
// in the Flowise UI (Variables page) — no SSH/redeploy needed to change it.
const unregisteredMarkupVarName = "UNREGISTERED_CUSTOMER_MARKUP"

// getUnregisteredMarkup reads the live markup percentage from Flowise's own
// "variable" table (go-crm shares the same Postgres database). Read fresh on
// every call rather than cached at startup, so an edit in the Flowise UI
// takes effect on the very next chat — no go-crm restart required.
//
// Falls back to the env-configured / hardcoded unregisteredCustomerMarkup if
// the Flowise Variable doesn't exist, isn't a valid number, or the query
// fails — the feature must keep working even if someone deletes the variable
// or the DB hiccups.
func getUnregisteredMarkup(ctx context.Context) float64 {
	var raw string
	err := pool.QueryRow(ctx,
		`SELECT value FROM variable WHERE name = $1 AND "workspaceId" = $2 LIMIT 1`,
		unregisteredMarkupVarName, flowiseWorkspaceID,
	).Scan(&raw)
	if err != nil {
		return unregisteredCustomerMarkup
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return unregisteredCustomerMarkup
	}
	return v
}
