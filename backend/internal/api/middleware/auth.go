package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// Claims holds the JWT payload fields KubePilot uses.
type Claims struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

// ContextKeyUserID is the context key for the authenticated user's ID.
const ContextKeyUserID = "user_id"

// ContextKeyEmail is the context key for the authenticated user's email.
const ContextKeyEmail = "user_email"

// ContextKeyRole is the context key for the authenticated user's role.
const ContextKeyRole = "user_role"

// JWTAuth returns a Gin middleware that validates Bearer JWT tokens.
// The secret must be the same value used when signing tokens.
func JWTAuth(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authorization header required"})
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization header format"})
			return
		}

		tokenStr := parts[1]
		claims := &Claims{}

		token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return []byte(secret), nil
		})

		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
			return
		}

		c.Set(ContextKeyUserID, claims.UserID)
		c.Set(ContextKeyEmail, claims.Email)
		c.Set(ContextKeyRole, claims.Role)
		c.Next()
	}
}

// RequireRole returns a middleware that enforces a minimum role level.
// Role hierarchy: admin > operator > viewer
func RequireRole(minRole string) gin.HandlerFunc {
	roleLevel := map[string]int{
		"viewer":   1,
		"operator": 2,
		"admin":    3,
	}

	return func(c *gin.Context) {
		role, _ := c.Get(ContextKeyRole)
		roleStr, _ := role.(string)

		if roleLevel[roleStr] < roleLevel[minRole] {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
			return
		}
		c.Next()
	}
}
