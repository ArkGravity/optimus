package image

import "time"

// Summary is one live image on a Docker host.
type Summary struct {
	ID       string    `json:"id"`
	ShortID  string    `json:"short_id"`
	RepoTags []string  `json:"repo_tags"`
	Dangling bool      `json:"dangling"`
	Size     int64     `json:"size"`
	Created  time.Time `json:"created"`
}

type ListResponse struct {
	Items []Summary `json:"items"`
	Total int       `json:"total"`
}

// DeleteResponse lists the tags removed and the image layers deleted.
type DeleteResponse struct {
	Untagged []string `json:"untagged"`
	Deleted  []string `json:"deleted"`
}

// PruneRequest selects dangling images only (default) or every image not
// used by any container (All).
type PruneRequest struct {
	All bool `json:"all"`
}

type PruneResponse struct {
	Untagged       []string `json:"untagged"`
	Deleted        []string `json:"deleted"`
	SpaceReclaimed uint64   `json:"space_reclaimed"`
}
