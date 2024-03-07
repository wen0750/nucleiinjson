package bytes

import (
	lib_bytes "github.com/wen0750/nucleiinjson/pkg/js/libs/bytes"

	"github.com/dop251/goja"
	"github.com/wen0750/nucleiinjson/pkg/js/gojs"
)

var (
	module = gojs.NewGojaModule("nuclei/bytes")
)

func init() {
	module.Set(
		gojs.Objects{
			// Functions
			"NewBuffer": lib_bytes.NewBuffer,

			// Var and consts

			// Types (value type)
			"Buffer": func() lib_bytes.Buffer { return lib_bytes.Buffer{} },

			// Types (pointer type)
		},
	).Register()
}

func Enable(runtime *goja.Runtime) {
	module.Enable(runtime)
}
