package auth

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/gadget-inc/dateilager/internal/db"
)

type ctxKey string

const (
	AuthCtxKey = ctxKey("auth")
)

type Role int

const (
	None Role = iota
	Project
	Admin
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
	default:
		return "unknown"
	}
}

var (
	noAuth    = Auth{Role: None}
	adminAuth = Auth{Role: Admin}
)

type AuthValidator struct {
	dbConn     db.DbConnector
	adminToken string
}

func NewAuthValidator(dbConn db.DbConnector) (*AuthValidator, error) {
	adminToken := os.Getenv("DL_ADMIN_TOKEN")

	if adminToken == "" {
		return nil, fmt.Errorf("missing DL_ADMIN_TOKEN")
	}

	return &AuthValidator{
		dbConn:     dbConn,
		adminToken: adminToken,
	}, nil
}

func (av *AuthValidator) Validate(ctx context.Context, fullToken string) (Auth, error) {
	if fullToken == av.adminToken {
		return adminAuth, nil
	}

	splits := strings.Split(fullToken, ":")
	project, err := strconv.ParseInt(splits[0], 10, 64)
	if err != nil || len(splits) != 2 {
		return noAuth, fmt.Errorf("missing project ID prefix in auth token")
	}

	token := splits[1]

	tx, close, err := av.dbConn.Connect(ctx)
	if err != nil {
		return noAuth, fmt.Errorf("auth db connect: %w", err)
	}
	defer close()

	var expectedToken string

	err = tx.QueryRow(ctx, `
		SELECT token
		FROM dl.projects
		WHERE id = $1
	`, project).Scan(&expectedToken)
	if err != nil {
		return noAuth, fmt.Errorf("auth reading project token: %w", err)
	}

	if expectedToken == token {
		return Auth{
			Role:    Project,
			Project: &project,
		}, nil
	}

	return noAuth, fmt.Errorf("invalid auth token for project %v", project)
}
