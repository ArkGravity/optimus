// Package gke implements explicit service-account authentication to GKE.
package gke

import "regexp"

const Context = "gke"

// Config identifies a cluster, never a user-supplied API or token endpoint.
type Config struct {
	Project  string `json:"project"`
	Location string `json:"location"`
	Cluster  string `json:"cluster"`
	Endpoint string `json:"endpoint" enums:"default,public,private,dns"`
}

var projectID = regexp.MustCompile(`^[a-z][a-z0-9-]{4,61}[a-z0-9]$|^[0-9]{1,20}$`)
var locationID = regexp.MustCompile(`^[a-z]+(?:-[a-z]+)+[0-9]+(?:-[a-z])?$`)
var clusterID = regexp.MustCompile(`^[a-z][a-z0-9-]{0,38}[a-z0-9]$|^[a-z]$`)

func (c Config) Valid() bool {
	return projectID.MatchString(c.Project) && len(c.Location) <= 64 && locationID.MatchString(c.Location) &&
		clusterID.MatchString(c.Cluster) && (c.Endpoint == "default" || c.Endpoint == "public" || c.Endpoint == "private" || c.Endpoint == "dns")
}
