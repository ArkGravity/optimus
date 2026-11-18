package image

import (
	"errors"
	"io"

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

func (h *Handler) HandleList() gin.HandlerFunc   { return h.list }
func (h *Handler) HandleDelete() gin.HandlerFunc { return h.delete }
func (h *Handler) HandlePrune() gin.HandlerFunc  { return h.prune }

func actor(c *gin.Context) uint64 { return c.GetUint64(middleware.CtxKeyUserID) }

type forceQuery struct {
	Force bool `form:"force"`
}

// list returns the live images on one Docker host.
// @Summary  List Docker images
// @Tags     docker
// @Security BearerAuth
// @Produce  json
// @Param    id path int true "host ID"
// @Success  200 {object} response.Envelope{data=ListResponse}
// @Router   /docker/hosts/{id}/images [get]
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

// delete removes one image by ID. In-use or multi-tag images need force.
// @Summary  Delete Docker image
// @Tags     docker
// @Security BearerAuth
// @Produce  json
// @Param    id      path  int    true  "host ID"
// @Param    imageId path  string true  "image ID (hex, optional sha256: prefix)"
// @Param    force   query bool   false "force removal of in-use or multi-tag images"
// @Success  200 {object} response.Envelope{data=DeleteResponse}
// @Router   /docker/hosts/{id}/images/{imageId} [delete]
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
	out, err := h.svc.Remove(c.Request.Context(), actor(c), c.ClientIP(), c.Request.UserAgent(), hostID, c.Param("imageId"), q.Force)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, out)
}

// prune removes dangling images, or every unused image when all is true.
// @Summary  Prune Docker images
// @Tags     docker
// @Security BearerAuth
// @Accept   json
// @Produce  json
// @Param    id   path int          true  "host ID"
// @Param    body body PruneRequest false "prune scope (default dangling only)"
// @Success  200 {object} response.Envelope{data=PruneResponse}
// @Router   /docker/hosts/{id}/images/prune [post]
func (h *Handler) prune(c *gin.Context) {
	hostID, ok := host.ParseID(c)
	if !ok {
		return
	}
	var req PruneRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		response.Error(c, apperr.New(apperr.CodeValidation, "common.validation", "invalid prune request"))
		return
	}
	out, err := h.svc.Prune(c.Request.Context(), actor(c), c.ClientIP(), c.Request.UserAgent(), hostID, req.All)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, out)
}
