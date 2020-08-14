package kv

import (
	"fmt"

	"github.com/cloudflare/cloudflare-go"
)

// PurgeTags purges the zone's cache by tags.
func PurgeTags(tags []string) error {
	resp, err := api.PurgeCache(zoneID, cloudflare.PurgeCacheRequest{
		Tags: tags,
	})
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf(fmt.Sprintf("purge tags fail: %v", resp))
	}
	return nil
}
