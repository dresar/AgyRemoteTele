package bot

import (
	"testing"
)

func TestBotRegisterCommandsSafeWhenNil(t *testing.T) {
	b := &Bot{}
	b.registerCommands()
}
