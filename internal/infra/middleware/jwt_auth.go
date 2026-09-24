package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/logic3579/optimus/internal/infra/crypto"
	apperr "github.com/logic3579/optimus/internal/infra/errors"
	"github.com/logic3579/optimus/internal/infra/response"
)

// JWTAuth validates the Authorization: Bearer <token> header. On success, sets
// the user_id in the context. On failure, aborts with 401 + envelope.
func JWTAuth(signer *crypto.JWTSigner) gin.HandlerFunc {
	return func(c *gin.Context) {
		hdr := c.GetHeader("Authorization")
		if !strings.HasPrefix(hdr, "Bearer ") {
			response.Error(c, apperr.New(apperr.CodeTokenInvalid, "auth.token_invalid", "missing or malformed Authorization header"))
			c.Abort()
			return
		}
		tok := strings.TrimPrefix(hdr, "Bearer ")
		claims, err := signer.Verify(tok)
		if err != nil {
			response.Error(c, apperr.New(apperr.CodeTokenInvalid, "auth.token_invalid", "invalid or expired token"))
			c.Abort()
			return
		}
		c.Set(CtxKeyUserID, claims.UserID)
		c.Next()
	}
}
