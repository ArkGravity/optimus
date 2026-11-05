package instance

import (
	"github.com/gin-gonic/gin"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/infra/permissions"
	"github.com/ArkGravity/optimus/internal/infra/response"
)

type Handler struct{ svc *Service }

func NewHandler(service *Service) *Handler { return &Handler{svc: service} }

func (h *Handler) Mount(group *gin.RouterGroup, requirePermission func(code string) gin.HandlerFunc) {
	read := group.Group("", requirePermission(permissions.PermAssetsResourceRead))
	read.GET("/instances", h.List)
	read.GET("/instances/:asset_key", h.Get)
}

// List returns a page of discovered instances.
// @Summary  List instances
// @Tags     assets
// @Security BearerAuth
// @Produce  json
// @Param    provider        query string false "cloud provider" Enums(aws,aliyun,gcp)
// @Param    project_id      query string false "GCP project"
// @Param    zone            query string false "zone"
// @Param    account_id      query int    false "filter by cloud account"
// @Param    region          query string false "filter by region"
// @Param    state           query string false "filter by instance state"
// @Param    vpc_id          query string false "filter by full native network reference"
// @Param    ip              query string false "exact private or public NIC IP address"
// @Param    tag_key         query string false "tag key"
// @Param    tag_value       query string false "tag value (requires tag_key)"
// @Param    q               query string false "search name, native ID, private IP, or tag values"
// @Param    include_deleted query bool   false "include soft-deleted instances" default(false)
// @Param    page            query int    false "page" default(1)
// @Param    size            query int    false "page size" default(20) maximum(200)
// @Success  200 {object} response.Envelope{data=ListResponse}
// @Failure  400 {object} response.Envelope
// @Failure  401 {object} response.Envelope
// @Failure  403 {object} response.Envelope
// @Failure  500 {object} response.Envelope
// @Router   /assets/instances [get]
func (h *Handler) List(c *gin.Context) {
	var query ListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.Error(c, apperr.New(apperr.CodeValidation, "common.validation", "invalid query parameters"))
		return
	}
	result, err := h.svc.List(c.Request.Context(), query)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, result)
}

// Get returns a discovered instance snapshot by canonical key or legacy AWS ID.
// @Summary  Get instance
// @Tags     assets
// @Security BearerAuth
// @Produce  json
// @Param    asset_key path string true "Canonical asset key or legacy AWS row ID"
// @Param    include_deleted query bool false "include a soft-deleted snapshot" default(false)
// @Success  200 {object} response.Envelope{data=Summary}
// @Failure  400 {object} response.Envelope
// @Failure  401 {object} response.Envelope
// @Failure  403 {object} response.Envelope
// @Failure  404 {object} response.Envelope
// @Failure  500 {object} response.Envelope
// @Router   /assets/instances/{asset_key} [get]
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
