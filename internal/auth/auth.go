package auth

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"strconv"

	"github.com/o1egl/paseto"
)

type ctxKey string

const (
	AuthCtxKey = ctxKey("auth")
)

type Role int

const (
	None         Role = iota
	Project           // read and write to one project
	Admin             // read and write to any project
	SharedReader      // read the shared caches, but no specific project data
)

type Auth struct {
	Role    Role
	Project *int64
}

func (a Auth) String() string {
	switch a.Role {
	case None:
		return "none"
	case Project:
		return fmt.Sprintf("project[%d]", *a.Project)
	case Admin:
		return "admin"
	case SharedReader:
		return "sharedReader"
	default:
		return "unknown"
	}
}

var (
	noAuth           = Auth{Role: None}
	adminAuth        = Auth{Role: Admin}
	sharedReaderAuth = Auth{Role: SharedReader}
)

type AuthValidator struct {
	pasetoKey ed25519.PublicKey
}

func NewAuthValidator(pasetoKey ed25519.PublicKey) *AuthValidator {
	return &AuthValidator{
		pasetoKey: pasetoKey,
	}
}

func (av *AuthValidator) Validate(ctx context.Context, token string) (Auth, error) {
	var payload paseto.JSONToken
	var footer string

	v2 := paseto.NewV2()

	err := v2.Verify(token, av.pasetoKey, &payload, &footer)
	if err != nil {
		return noAuth, fmt.Errorf("verify token %v: %w", token, err)
	}

	if payload.Subject == "admin" {
		return adminAuth, nil
	}

	if payload.Subject == "shared-reader" {
		return sharedReaderAuth, nil
	}

	project, err := strconv.ParseInt(payload.Subject, 10, 64)
	if err != nil {
		return noAuth, fmt.Errorf("parse Paseto subject %v: %w", payload.Subject, err)
	}

	return Auth{
		Role:    Project,
		Project: &project,
	}, nil
}
