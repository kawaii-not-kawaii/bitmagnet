package auth

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/bitmagnet-io/bitmagnet/internal/config/configapply"
	server "github.com/bitmagnet-io/bitmagnet/internal/httpserver"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

var errAlreadyConfigured = errors.New("already configured")

type httpBuilder struct {
	authenticator *Authenticator
	applier       *configapply.Applier
}

func NewHTTPServer(authenticator *Authenticator, applier *configapply.Applier) server.Option {
	return httpBuilder{authenticator: authenticator, applier: applier}
}

func (httpBuilder) Key() string {
	return "auth"
}

func (builder httpBuilder) Apply(engine *gin.Engine) error {
	engine.GET("/auth/state", builder.state)
	engine.POST("/auth/setup", builder.setup)
	engine.POST("/auth/login", builder.login)
	engine.POST("/auth/logout", builder.logout)

	return nil
}

func (builder httpBuilder) state(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, gin.H{
		"authDisabled":  builder.authenticator.Disabled(),
		"needsSetup":    builder.authenticator.NeedsSetup(),
		"trustedBypass": builder.authenticator.TrustedBypass(ctx.Request),
	})
}

type setupRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (builder httpBuilder) setup(ctx *gin.Context) {
	if builder.authenticator.Disabled() || !builder.authenticator.NeedsSetup() {
		ctx.JSON(http.StatusConflict, gin.H{"error": errAlreadyConfigured.Error()})
		return
	}

	var request setupRequest
	if err := ctx.ShouldBindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	if request.Username == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "username is required"})
		return
	}

	if len(request.Password) < 8 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "password must be at least 8 characters"})
		return
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(request.Password), bcrypt.DefaultCost)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid password"})
		return
	}

	cfg := builder.authenticator.config()
	cfg.Username = request.Username
	cfg.PasswordHash = string(passwordHash)

	_, err = builder.applier.SetSectionPrivileged("auth", cfg, func(current any) error {
		currentConfig, ok := current.(Config)
		if !ok {
			return fmt.Errorf("expected %T, got %T", Config{}, current)
		}

		if currentConfig.Disabled || (currentConfig.Username != "" && currentConfig.PasswordHash != "") {
			return errAlreadyConfigured
		}

		return nil
	})
	if errors.Is(err, errAlreadyConfigured) {
		ctx.JSON(http.StatusConflict, gin.H{"error": errAlreadyConfigured.Error()})
		return
	}

	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to persist credentials"})
		return
	}

	builder.authenticator.SetSessionCookie(ctx.Writer, ctx.Request)
	ctx.JSON(http.StatusOK, gin.H{"ok": true})
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	APIKey   string `json:"apiKey"`
}

func (builder httpBuilder) login(ctx *gin.Context) {
	if builder.authenticator.Disabled() {
		ctx.JSON(http.StatusOK, gin.H{"ok": true, "authDisabled": true})
		return
	}

	var request loginRequest
	if err := ctx.ShouldBindJSON(&request); err != nil {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	valid := builder.authenticator.ValidatePassword(request.Username, request.Password)
	if request.APIKey != "" {
		valid = builder.authenticator.ValidateAPIKey(request.APIKey)
	}

	if !valid {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	builder.authenticator.SetSessionCookie(ctx.Writer, ctx.Request)
	ctx.JSON(http.StatusOK, gin.H{"ok": true})
}

func (builder httpBuilder) logout(ctx *gin.Context) {
	builder.authenticator.ClearSessionCookie(ctx.Writer, ctx.Request)
	ctx.Status(http.StatusNoContent)
}
