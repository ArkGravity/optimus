package storage

import (
	"strconv"

	"github.com/gin-gonic/gin"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/infra/response"
)

type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

// List godoc
// @Summary List read-only Kubernetes storage resources
// @Tags k8s
// @Produce json
// @Security BearerAuth
// @Param id path int true "Cluster ID"
// @Param kind path string true "Resource kind" Enums(persistentvolumes,persistentvolumeclaims,storageclasses)
// @Param namespace query string false "PVC namespace; empty lists all namespaces"
// @Param limit query int false "Page limit (default 500, maximum 2000)"
// @Param continue query string false "Kubernetes continuation token"
// @Success 200 {object} response.Envelope{data=ListResponse}
// @Router /k8s/clusters/{id}/storage/{kind} [get]
func (h *Handler) List() gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil || id == 0 {
			response.Error(c, apperr.New(apperr.CodeBadRequest, "common.bad_request", "invalid cluster id"))
			return
		}
		var query ListQuery
		if err := c.ShouldBindQuery(&query); err != nil {
			response.Error(c, apperr.New(apperr.CodeBadRequest, "common.bad_request", "invalid list query"))
			return
		}
		out, err := h.service.List(c.Request.Context(), id, c.Param("kind"), query)
		if err != nil {
			response.Error(c, err)
			return
		}
		response.Success(c, out)
	}
}
