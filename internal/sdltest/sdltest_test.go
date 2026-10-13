package sdltest_test

import (
	"os"
	"testing"

	"github.com/opengoui/puresdl/internal/sdltest"
)

func TestMain(m *testing.M) { os.Exit(sdltest.Main(m)) }
