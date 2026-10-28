package securitygroup

import (
	"strconv"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/infra/response"
	"github.com/gin-gonic/gin"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }
func (h *Handler) Mount(group *gin.RouterGroup, permission func(string) gin.HandlerFunc) {
	read := group.Group("", permission("assets:resource:read"))
	read.GET("/security-groups", h.List)
	read.GET("/security-groups/:id", h.Get)
}

// List returns security group snapshots.
// @Summary List security groups
// @Tags assets
// @Security BearerAuth
// @Produce json
// @Param account_id query int false "cloud account ID"
// @Param region query string false "AWS region"
// @Param vpc_id query string false "VPC ID"
// @Param q query string false "name, group ID, or description"
// @Param include_deleted query bool false "include deleted snapshots"
// @Param page query int false "page" default(1)
// @Param size query int false "page size" default(20) maximum(200)
// @Success 200 {object} response.Envelope{data=ListResponse}
// @Failure 400 {object} response.Envelope
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Failure 500 {object} response.Envelope
// @Router /assets/security-groups [get]
func (h *Handler) List(c *gin.Context) {
	var q ListQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		response.Error(c, apperr.New(apperr.CodeValidation, "common.validation", "invalid query parameters"))
		return
	}
	result, err := h.svc.List(c.Request.Context(), q)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, result)
}

// Get returns one security group with inbound and outbound rules.
// @Summary Get security group rules
// @Tags assets
// @Security BearerAuth
// @Produce json
// @Param id path int true "security group snapshot row ID"
// @Success 200 {object} response.Envelope{data=Detail}
// @Failure 400 {object} response.Envelope
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Failure 404 {object} response.Envelope
// @Failure 500 {object} response.Envelope
// @Router /assets/security-groups/{id} [get]
func (h *Handler) Get(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 63)
	if err != nil || id == 0 {
		response.Error(c, apperr.New(apperr.CodeValidation, "common.validation", "invalid security group id"))
		return
	}
	result, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, result)
}
