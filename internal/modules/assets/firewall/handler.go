package firewall

import (
	"github.com/gin-gonic/gin"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/infra/permissions"
	"github.com/ArkGravity/optimus/internal/infra/response"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }
func (h *Handler) Mount(group *gin.RouterGroup, requirePermission func(string) gin.HandlerFunc) {
	read := group.Group("", requirePermission(permissions.PermAssetsResourceRead))
	read.GET("/firewalls", h.List)
	read.GET("/firewalls/:asset_key", h.Get)
}

// List returns summaries of discovered firewall resources; counts mean displayed entries.
// @Summary List firewalls
// @Tags assets
// @Security BearerAuth
// @Produce json
// @Param provider query string false "cloud provider" Enums(aws,aliyun,gcp)
// @Param account_id query int false "cloud account"
// @Param project_id query string false "GCP project"
// @Param region query string false "region"
// @Param zone query string false "zone"
// @Param network_ref query string false "full native network reference"
// @Param q query string false "search name, native ID, description or tags"
// @Param include_deleted query bool false "include soft-deleted resources" default(false)
// @Param page query int false "page" default(1)
// @Param size query int false "page size" default(20) maximum(200)
// @Success 200 {object} response.Envelope{data=ListResponse}
// @Failure 400 {object} response.Envelope
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Failure 500 {object} response.Envelope
// @Router /assets/firewalls [get]
func (h *Handler) List(c *gin.Context) {
	var input ListQuery
	if err := c.ShouldBindQuery(&input); err != nil {
		response.Error(c, apperr.New(apperr.CodeValidation, "common.validation", "invalid query parameters"))
		return
	}
	result, err := h.svc.List(c.Request.Context(), input)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, result)
}

// Get returns common displayed rules and typed provider facts for a firewall.
// @Summary Get firewall
// @Tags assets
// @Security BearerAuth
// @Produce json
// @Param asset_key path string true "Canonical asset key or legacy AWS row ID"
// @Param include_deleted query bool false "include a soft-deleted snapshot" default(false)
// @Success 200 {object} response.Envelope{data=Detail}
// @Failure 400 {object} response.Envelope
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Failure 404 {object} response.Envelope
// @Failure 500 {object} response.Envelope
// @Router /assets/firewalls/{asset_key} [get]
func (h *Handler) Get(c *gin.Context) {
	var input struct {
		IncludeDeleted bool `form:"include_deleted"`
	}
	if err := c.ShouldBindQuery(&input); err != nil {
		response.Error(c, apperr.New(apperr.CodeValidation, "common.validation", "invalid query parameters"))
		return
	}
	result, err := h.svc.FindByKey(c.Request.Context(), c.Param("asset_key"), input.IncludeDeleted)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, result)
}
