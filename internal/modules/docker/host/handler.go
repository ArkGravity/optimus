package host

import (
	"strconv"

	"github.com/gin-gonic/gin"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/infra/middleware"
	"github.com/ArkGravity/optimus/internal/infra/response"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) HandleList() gin.HandlerFunc   { return h.list }
func (h *Handler) HandleGet() gin.HandlerFunc    { return h.get }
func (h *Handler) HandleCreate() gin.HandlerFunc { return h.create }
func (h *Handler) HandleUpdate() gin.HandlerFunc { return h.update }
func (h *Handler) HandleDelete() gin.HandlerFunc { return h.delete }
func (h *Handler) HandleTest() gin.HandlerFunc   { return h.test }

func actor(c *gin.Context) uint64 { return c.GetUint64(middleware.CtxKeyUserID) }

// ParseID reads the :id path parameter, writing a 40001 envelope on failure.
func ParseID(c *gin.Context) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Error(c, apperr.New(apperr.CodeBadRequest, "common.bad_request", "invalid id"))
		return 0, false
	}
	return id, true
}

// list returns a paginated set of Docker hosts.
// @Summary  List Docker hosts
// @Tags     docker
// @Security BearerAuth
// @Produce  json
// @Param    page      query int    false "page (default 1)"
// @Param    page_size query int    false "page size (default 20)"
// @Param    search    query string false "name / description / host substring"
// @Success  200 {object} response.Envelope
// @Router   /docker/hosts [get]
func (h *Handler) list(c *gin.Context) {
	var q ListQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		response.Error(c, apperr.New(apperr.CodeValidation, "common.validation", "invalid query parameters"))
		return
	}
	out, err := h.svc.List(c.Request.Context(), q)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, out)
}

// get returns one Docker host.
// @Summary  Get Docker host
// @Tags     docker
// @Security BearerAuth
// @Produce  json
// @Param    id path int true "host ID"
// @Success  200 {object} response.Envelope
// @Router   /docker/hosts/{id} [get]
func (h *Handler) get(c *gin.Context) {
	id, ok := ParseID(c)
	if !ok {
		return
	}
	out, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, out)
}

// create stores a new Docker host. Passwords are sealed and never returned.
// @Summary  Create Docker host
// @Tags     docker
// @Security BearerAuth
// @Accept   json
// @Produce  json
// @Param    body body CreateRequest true "host payload"
// @Success  200 {object} response.Envelope
// @Router   /docker/hosts [post]
func (h *Handler) create(c *gin.Context) {
	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperr.New(apperr.CodeValidation, "common.validation", err.Error()))
		return
	}
	out, err := h.svc.Create(c.Request.Context(), actor(c), c.ClientIP(), c.Request.UserAgent(), req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, out)
}

// update patches a Docker host. An omitted password keeps the stored one.
// @Summary  Update Docker host
// @Tags     docker
// @Security BearerAuth
// @Accept   json
// @Produce  json
// @Param    id   path int           true "host ID"
// @Param    body body UpdateRequest true "host payload"
// @Success  200 {object} response.Envelope
// @Router   /docker/hosts/{id} [put]
func (h *Handler) update(c *gin.Context) {
	id, ok := ParseID(c)
	if !ok {
		return
	}
	var req UpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperr.New(apperr.CodeValidation, "common.validation", err.Error()))
		return
	}
	out, err := h.svc.Update(c.Request.Context(), actor(c), c.ClientIP(), c.Request.UserAgent(), id, req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, out)
}

// delete soft-deletes a Docker host and discards its sealed password.
// @Summary  Delete Docker host
// @Tags     docker
// @Security BearerAuth
// @Produce  json
// @Param    id path int true "host ID"
// @Success  200 {object} response.Envelope
// @Router   /docker/hosts/{id} [delete]
func (h *Handler) delete(c *gin.Context) {
	id, ok := ParseID(c)
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), actor(c), c.ClientIP(), c.Request.UserAgent(), id); err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, gin.H{"ok": true})
}

// test opens an SSH tunnel to the host's Docker socket, queries the Engine
// version, and persists the outcome as the host's last check.
// @Summary  Test Docker host connection
// @Tags     docker
// @Security BearerAuth
// @Produce  json
// @Param    id path int true "host ID"
// @Success  200 {object} response.Envelope
// @Router   /docker/hosts/{id}/test [post]
func (h *Handler) test(c *gin.Context) {
	id, ok := ParseID(c)
	if !ok {
		return
	}
	out, err := h.svc.Test(c.Request.Context(), actor(c), c.ClientIP(), c.Request.UserAgent(), id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, out)
}
