// Package docker is the composition root for Docker management: SSH-connected
// host metadata plus live Engine operations over each host's Unix socket.
package docker

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/ArkGravity/optimus/internal/infra/middleware"
	"github.com/ArkGravity/optimus/internal/modules/audit"
	"github.com/ArkGravity/optimus/internal/modules/credentials"
	"github.com/ArkGravity/optimus/internal/modules/docker/container"
	"github.com/ArkGravity/optimus/internal/modules/docker/host"
	"github.com/ArkGravity/optimus/internal/modules/docker/image"
	"github.com/ArkGravity/optimus/internal/modules/rbac"
)

// Module bundles the Docker services and handlers for one MountRoutes call.
type Module struct {
	// Host exposes host CRUD and Connect for live Engine verticals.
	Host      *host.Service
	Image     *image.Service
	Container *container.Service

	hostHandler      *host.Handler
	imageHandler     *image.Handler
	containerHandler *container.Handler
}

// New wires the module. cipher is the shared vault cipher; consumer is the
// credentials seam used to read SSH keys.
func New(db *gorm.DB, cipher host.Cipher, consumer credentials.Consumer, rec *audit.Recorder) *Module {
	hostSvc := host.NewService(host.NewRepo(db), cipher, consumer, nil, rec)
	imageSvc := image.NewService(hostSvc.Connect, rec)
	containerSvc := container.NewService(hostSvc.Connect, rec)
	return &Module{
		Host:             hostSvc,
		Image:            imageSvc,
		Container:        containerSvc,
		hostHandler:      host.NewHandler(hostSvc),
		imageHandler:     image.NewHandler(imageSvc),
		containerHandler: container.NewHandler(containerSvc),
	}
}

// MountRoutes registers /docker routes under the JWT-protected group.
// Connection tests only refresh probe metadata, so they share the read gate.
func (m *Module) MountRoutes(protected *gin.RouterGroup, cache *rbac.PermissionCache) {
	hosts := protected.Group("/docker/hosts")

	rd := hosts.Group("", middleware.RequirePermission(cache, "docker:environment:read"))
	rd.GET("", m.hostHandler.HandleList())
	rd.GET("/:id", m.hostHandler.HandleGet())
	rd.POST("/:id/test", m.hostHandler.HandleTest())
	rd.GET("/:id/images", m.imageHandler.HandleList())
	rd.GET("/:id/containers", m.containerHandler.HandleList())
	rd.GET("/:id/containers/:containerId", m.containerHandler.HandleGet())

	wr := hosts.Group("", middleware.RequirePermission(cache, "docker:environment:write"))
	wr.POST("", m.hostHandler.HandleCreate())
	wr.PUT("/:id", m.hostHandler.HandleUpdate())
	wr.DELETE("/:id", m.hostHandler.HandleDelete())
	wr.DELETE("/:id/images/:imageId", m.imageHandler.HandleDelete())
	wr.POST("/:id/images/prune", m.imageHandler.HandlePrune())
	wr.POST("/:id/containers/:containerId/start", m.containerHandler.HandleStart())
	wr.POST("/:id/containers/:containerId/stop", m.containerHandler.HandleStop())
	wr.POST("/:id/containers/:containerId/restart", m.containerHandler.HandleRestart())
	wr.DELETE("/:id/containers/:containerId", m.containerHandler.HandleDelete())
}
