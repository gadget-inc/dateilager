package environment

import (
	"fmt"
	"math"
	"os"
	"strconv"
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

func Load() (Env, error) {
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

func LoadOrProduction() Env {
	env, err := Load()
	if err != nil {
		return Prod
	}
	return env
}

// EnvInt reads key from the environment, parses it as an integer, and
// multiplies by multiplier. Returns (0, false) when the variable is unset,
// cannot be parsed, or the parsed value is not positive.
func EnvInt(key string, multiplier int) (int, bool) {
	v := os.Getenv(key)
	if v == "" {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n * multiplier, true
}

// EnvInt32 behaves like EnvInt but additionally guards that the result fits
// in an int32 before returning.
func EnvInt32(key string, multiplier int) (int32, bool) {
	v := os.Getenv(key)
	if v == "" {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 0, false
	}
	result := n * multiplier
	if result > math.MaxInt32 {
		return 0, false
	}
	return int32(result), true
}
