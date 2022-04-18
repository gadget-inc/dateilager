package environment

import (
	"fmt"
	"os"
	"strings"
)

type Env int

const (
	// Use iota + 1 to ensure that a default Env value is invalid and not Dev
	// We do not want fs.Reset() to be available if the user forgot to set the app's Env
	Dev Env = iota + 1
	Test
	Prod
)

func (e Env) String() string {
	switch e {
	case Dev:
		return "dev"
	case Test:
		return "test"
	case Prod:
		return "prod"
	default:
		return "unknown"
	}
}

func LoadEnvironment() (Env, error) {
	envStr := os.Getenv("DL_ENV")

	switch strings.ToLower(envStr) {
	case "dev":
		return Dev, nil
	case "test":
		return Test, nil
	case "prod":
		return Prod, nil
	default:
		return 0, fmt.Errorf("unknown environment: %s", envStr)
	}
}
