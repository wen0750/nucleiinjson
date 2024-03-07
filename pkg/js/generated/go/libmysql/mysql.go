package mysql

import (
	lib_mysql "github.com/wen0750/nucleiinjson/pkg/js/libs/mysql"

	"github.com/dop251/goja"
	"github.com/wen0750/nucleiinjson/pkg/js/gojs"
)

var (
	module = gojs.NewGojaModule("nuclei/mysql")
)

func init() {
	module.Set(
		gojs.Objects{
			// Functions

			// Var and consts

			// Types (value type)
			"MySQLClient": func() lib_mysql.MySQLClient { return lib_mysql.MySQLClient{} },

			// Types (pointer type)
			"NewMySQLClient": func() *lib_mysql.MySQLClient { return &lib_mysql.MySQLClient{} },
		},
	).Register()
}

func Enable(runtime *goja.Runtime) {
	module.Enable(runtime)
}
