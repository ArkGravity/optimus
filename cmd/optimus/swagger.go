package main

import (
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	apperr "github.com/ArkGravity/optimus/internal/infra/errors"
	"github.com/ArkGravity/optimus/internal/infra/response"
)

func mountSwaggerRoutes(r *gin.Engine) {
	handler := ginSwagger.WrapHandler(swaggerFiles.NewHandler())
	r.GET("/swagger/*any", func(c *gin.Context) {
		// These are the resources supported by the pinned gin-swagger handler.
		// Its unanchored matcher also accepts unknown prefixes and suffixes.
		switch c.Param("any") {
		case "/index.html", "/doc.json", "/oauth2-redirect.html",
			"/favicon-16x16.png", "/favicon-32x32.png",
			"/swagger-ui.css", "/swagger-ui.css.map",
			"/swagger-ui.js", "/swagger-ui.js.map",
			"/swagger-ui-bundle.js", "/swagger-ui-bundle.js.map",
			"/swagger-ui-standalone-preset.js", "/swagger-ui-standalone-preset.js.map":
		default:
			response.Error(c, apperr.New(apperr.CodeNotFound, "common.not_found", "route not found"))
			return
		}

		// The upstream matcher reads RequestURI. Match only the validated path
		// so query values cannot select a different resource or handler prefix.
		requestURI := c.Request.RequestURI
		c.Request.RequestURI = c.Request.URL.Path
		defer func() { c.Request.RequestURI = requestURI }()
		handler(c)
	})
}
