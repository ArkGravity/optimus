package resource

import "errors"

// Location keeps global, regional and zonal resource scopes distinct.
type Location struct {
	Type   string `json:"location_type"`
	Name   string `json:"location"`
	Region string `json:"region"`
	Zone   string `json:"zone"`
}

func (l Location) Validate() error {
	switch l.Type {
	case "global":
		if l.Name == "global" && l.Region == "" && l.Zone == "" {
			return nil
		}
	case "region":
		if l.Region != "" && l.Name == l.Region && l.Zone == "" {
			return nil
		}
	case "zone":
		if l.Region != "" && l.Zone != "" && l.Name == l.Zone {
			return nil
		}
	}
	return errors.New("invalid asset location")
}
