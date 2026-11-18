package container

import (
	"github.com/gin-gonic/gin"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/infra/middleware"
	"github.com/ArkGravity/optimus/internal/infra/response"
	"github.com/ArkGravity/optimus/internal/modules/docker/host"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) HandleList() gin.HandlerFunc    { return h.list }
func (h *Handler) HandleGet() gin.HandlerFunc     { return h.get }
func (h *Handler) HandleStart() gin.HandlerFunc   { return h.start }
func (h *Handler) HandleStop() gin.HandlerFunc    { return h.stop }
func (h *Handler) HandleRestart() gin.HandlerFunc { return h.restart }
func (h *Handler) HandleDelete() gin.HandlerFunc  { return h.delete }

func actor(c *gin.Context) uint64 { return c.GetUint64(middleware.CtxKeyUserID) }

type forceQuery struct {
	Force bool `form:"force"`
}

// list returns every container on one Docker host, including stopped ones.
// @Summary  List Docker containers
// @Tags     docker
// @Security BearerAuth
// @Produce  json
// @Param    id path int true "host ID"
// @Success  200 {object} response.Envelope{data=ListResponse}
// @Router   /docker/hosts/{id}/containers [get]
func (h *Handler) list(c *gin.Context) {
	hostID, ok := host.ParseID(c)
	if !ok {
		return
	}
	out, err := h.svc.List(c.Request.Context(), actor(c), hostID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, out)
}

// get returns the inspect summary of one container.
// @Summary  Get Docker container
// @Tags     docker
// @Security BearerAuth
// @Produce  json
// @Param    id          path int    true "host ID"
// @Param    containerId path string true "container ID or name"
// @Success  200 {object} response.Envelope{data=Detail}
// @Router   /docker/hosts/{id}/containers/{containerId} [get]
func (h *Handler) get(c *gin.Context) {
	hostID, ok := host.ParseID(c)
	if !ok {
		return
	}
	out, err := h.svc.Get(c.Request.Context(), actor(c), hostID, c.Param("containerId"))
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, out)
}

// start starts a container.
// @Summary  Start Docker container
// @Tags     docker
// @Security BearerAuth
// @Produce  json
// @Param    id          path int    true "host ID"
// @Param    containerId path string true "container ID or name"
// @Success  200 {object} response.Envelope
// @Router   /docker/hosts/{id}/containers/{containerId}/start [post]
func (h *Handler) start(c *gin.Context) { h.do(c, ActionStart) }

// stop stops a container with the Engine's default grace period.
// @Summary  Stop Docker container
// @Tags     docker
// @Security BearerAuth
// @Produce  json
// @Param    id          path int    true "host ID"
// @Param    containerId path string true "container ID or name"
// @Success  200 {object} response.Envelope
// @Router   /docker/hosts/{id}/containers/{containerId}/stop [post]
func (h *Handler) stop(c *gin.Context) { h.do(c, ActionStop) }

// restart restarts a container with the Engine's default grace period.
// @Summary  Restart Docker container
// @Tags     docker
// @Security BearerAuth
// @Produce  json
// @Param    id          path int    true "host ID"
// @Param    containerId path string true "container ID or name"
// @Success  200 {object} response.Envelope
// @Router   /docker/hosts/{id}/containers/{containerId}/restart [post]
func (h *Handler) restart(c *gin.Context) { h.do(c, ActionRestart) }

func (h *Handler) do(c *gin.Context, action Action) {
	hostID, ok := host.ParseID(c)
	if !ok {
		return
	}
	if err := h.svc.Do(c.Request.Context(), actor(c), c.ClientIP(), c.Request.UserAgent(), hostID, c.Param("containerId"), action); err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, gin.H{"ok": true})
}

// delete removes a container. Running containers need force.
// @Summary  Delete Docker container
// @Tags     docker
// @Security BearerAuth
// @Produce  json
// @Param    id          path  int    true  "host ID"
// @Param    containerId path  string true  "container ID or name"
// @Param    force       query bool   false "kill and remove a running container"
// @Success  200 {object} response.Envelope
// @Router   /docker/hosts/{id}/containers/{containerId} [delete]
func (h *Handler) delete(c *gin.Context) {
	hostID, ok := host.ParseID(c)
	if !ok {
		return
	}
	var q forceQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		response.Error(c, apperr.New(apperr.CodeValidation, "common.validation", "invalid query parameters"))
		return
	}
	if err := h.svc.Remove(c.Request.Context(), actor(c), c.ClientIP(), c.Request.UserAgent(), hostID, c.Param("containerId"), q.Force); err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, gin.H{"ok": true})
}
