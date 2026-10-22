package app

import "maps"

func contentEntries(videos, dynamics []Entry) map[string]bool {
	entries := map[string]bool{}
	for kind, items := range map[string][]Entry{"video": videos, "image_post": dynamics} {
		for _, entry := range items {
			if entry.ID != "" {
				entries[kind+":"+entry.ID] = true
			}
		}
	}
	return entries
}

func mergeCatalog(catalog *map[string]bool, entries map[string]bool) {
	if *catalog == nil {
		*catalog = map[string]bool{}
	}
	maps.Copy(*catalog, entries)
}

func (a *App) recordContentCatalog(creatorID string, feed bilibiliFeed) error {
	return a.transaction(func() error {
		for i := range a.state.Creators {
			c := &a.state.Creators[i]
			if c.ID != creatorID || !c.Enabled {
				continue
			}
			entries := contentEntries(feed.Videos, feed.Dynamics)
			if h := c.HistoryJob; h != nil && h.CatalogFromStart && (h.Status == "running" || h.Status == "paused" || h.Status == "failed") {
				// 历史末页替换目录时，保留分页期间普通监听发现的新内容。
				mergeCatalog(&h.CatalogEntries, entries)
			}
			if feed.Complete {
				c.ContentCatalog = entries
				c.ContentCatalogComplete = true
			} else {
				mergeCatalog(&c.ContentCatalog, entries)
			}
			break
		}
		return nil
	})
}
