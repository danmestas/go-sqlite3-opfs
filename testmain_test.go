//go:build js

package opfsvfs

import (
	"os"
	"testing"

	"github.com/danmestas/go-sqlite3-opfs/testharness"
)

func TestMain(m *testing.M) {
	<-initReady
	code := m.Run()
	testharness.PostDone(code == 0, "")
	os.Exit(code)
}
