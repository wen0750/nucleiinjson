package smb

import (
	lib_smb "github.com/wen0750/nucleiinjson/pkg/js/libs/smb"

	"github.com/dop251/goja"
	"github.com/wen0750/nucleiinjson/pkg/js/gojs"
)

var (
	module = gojs.NewGojaModule("nuclei/smb")
)

func init() {
	module.Set(
		gojs.Objects{
			// Functions

			// Var and consts

			// Types (value type)
			"SMBClient": func() lib_smb.SMBClient { return lib_smb.SMBClient{} },

			// Types (pointer type)
			"NewSMBClient": func() *lib_smb.SMBClient { return &lib_smb.SMBClient{} },
		},
	).Register()
}

func Enable(runtime *goja.Runtime) {
	module.Enable(runtime)
}
