package manager_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/telepresenceio/telepresence/v2/cmd/traffic/cmd/manager"
)

func TestEnvconfig(t *testing.T) {
	origEnv := os.Environ()
	defer func() {
		os.Clearenv()
		for _, kv := range origEnv {
			eq := strings.Index(kv, "=")
			if eq < 0 {
				continue
			}
			k := kv[:eq]
			v := kv[eq+1:]
			os.Setenv(k, v)
		}
	}()

	defaults := manager.Env{
		User:        "",
		ServerHost:  "",
		ServerPort:  "8081",
		SystemAHost: "app.getambassador.io",
		SystemAPort: "443",
	}

	testcases := map[string]struct {
		Input  map[string]string
		Output func(*manager.Env)
	}{
		"empty": {
			Input:  nil,
			Output: func(*manager.Env) {},
		},
		"simple": {
			Input: map[string]string{
				"SYSTEMA_HOST": "app.getambassador.io",
			},
			Output: func(e *manager.Env) {
				e.SystemAHost = "app.getambassador.io"
			},
		},
	}

	for tcName, tc := range testcases {
		tc := tc // Capture loop variable...
		// Because we don't run the subtests in parallel, capturing the loop variable
		// doesn't really matter, but scopelint complains.

		t.Run(tcName, func(t *testing.T) {
			assert := assert.New(t)

			os.Clearenv()
			for k, v := range tc.Input {
				os.Setenv(k, v)
			}

			expected := defaults
			tc.Output(&expected)

			actual, err := manager.LoadEnv(context.Background())
			assert.Nil(err)
			assert.Equal(expected, actual)
		})
	}
}
