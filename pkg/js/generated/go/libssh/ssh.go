package ssh

import (
	lib_ssh "github.com/wen0750/nucleiinjson/pkg/js/libs/ssh"

	"github.com/dop251/goja"
	"github.com/wen0750/nucleiinjson/pkg/js/gojs"
)

var (
	module = gojs.NewGojaModule("nuclei/ssh")
)

func init() {
	module.Set(
		gojs.Objects{
			// Functions

			// Var and consts

			// Types (value type)
			"SSHClient": func() lib_ssh.SSHClient { return lib_ssh.SSHClient{} },

			// Types (pointer type)
			"NewSSHClient": func() *lib_ssh.SSHClient { return &lib_ssh.SSHClient{} },
		},
	).Register()
}

func Enable(runtime *goja.Runtime) {
	module.Enable(runtime)
}
