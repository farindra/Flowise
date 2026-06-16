// Package meili wraps the Meilisearch SDK for the "products" index used by
// product-search-service.
package meili

import (
	"context"
	"fmt"
	"log"

	meilisearch "github.com/meilisearch/meilisearch-go"

	"sync-indexer/internal/transform"
)

const IndexUID = "products"

type Client struct {
	sm meilisearch.ServiceManager
}

func New(url, apiKey string) *Client {
	return &Client{sm: meilisearch.New(url, meilisearch.WithAPIKey(apiKey))}
}

// EnsureIndex creates the "products" index (primary key "id") if it doesn't
// exist yet, and applies searchable/filterable attribute settings
// idempotently. code_variations is both searchable (fuzzy fallback) and
// filterable (product-search-service's exact-match phase).
func (c *Client) EnsureIndex(ctx context.Context) error {
	if _, err := c.sm.CreateIndexWithContext(ctx, &meilisearch.IndexConfig{
		Uid:        IndexUID,
		PrimaryKey: "id",
	}); err != nil {
		log.Printf("meili: create index %q: %v (ok if it already exists)", IndexUID, err)
	}

	idx := c.sm.Index(IndexUID)
	task, err := idx.UpdateSettingsWithContext(ctx, &meilisearch.Settings{
		SearchableAttributes: []string{"kode", "code_variations", "nama", "brand"},
		FilterableAttributes: []string{"stok", "active", "available", "code_variations"},
	})
	if err != nil {
		return fmt.Errorf("update settings: %w", err)
	}
	if _, err := c.sm.WaitForTaskWithContext(ctx, task.TaskUID, 0); err != nil {
		return fmt.Errorf("wait for settings task: %w", err)
	}
	return nil
}

// DocumentCount returns the current number of documents in the index.
func (c *Client) DocumentCount(ctx context.Context) (int64, error) {
	stats, err := c.sm.Index(IndexUID).GetStatsWithContext(ctx, nil)
	if err != nil {
		return 0, err
	}
	return stats.NumberOfDocuments, nil
}

// ReplaceAllDocuments deletes all existing documents and adds the new set,
// waiting for both tasks to complete.
func (c *Client) ReplaceAllDocuments(ctx context.Context, docs []transform.ProductDoc) error {
	idx := c.sm.Index(IndexUID)

	delTask, err := idx.DeleteAllDocumentsWithContext(ctx, nil)
	if err != nil {
		return fmt.Errorf("delete all documents: %w", err)
	}
	if _, err := c.sm.WaitForTaskWithContext(ctx, delTask.TaskUID, 0); err != nil {
		return fmt.Errorf("wait for delete task: %w", err)
	}

	addTask, err := idx.AddDocumentsWithContext(ctx, docs, nil)
	if err != nil {
		return fmt.Errorf("add documents: %w", err)
	}
	if _, err := c.sm.WaitForTaskWithContext(ctx, addTask.TaskUID, 0); err != nil {
		return fmt.Errorf("wait for add documents task: %w", err)
	}
	return nil
}
