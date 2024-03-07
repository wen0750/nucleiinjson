package protocolinit

import (
	"github.com/wen0750/nucleiinjson/pkg/js/compiler"
	"github.com/wen0750/nucleiinjson/pkg/protocols/common/protocolstate"
	"github.com/wen0750/nucleiinjson/pkg/protocols/dns/dnsclientpool"
	"github.com/wen0750/nucleiinjson/pkg/protocols/http/httpclientpool"
	"github.com/wen0750/nucleiinjson/pkg/protocols/http/signerpool"
	"github.com/wen0750/nucleiinjson/pkg/protocols/network/networkclientpool"
	"github.com/wen0750/nucleiinjson/pkg/protocols/whois/rdapclientpool"
	"github.com/wen0750/nucleiinjson/pkg/types"
)

// Init initializes the client pools for the protocols
func Init(options *types.Options) error {

	if err := protocolstate.Init(options); err != nil {
		return err
	}
	if err := dnsclientpool.Init(options); err != nil {
		return err
	}
	if err := httpclientpool.Init(options); err != nil {
		return err
	}
	if err := signerpool.Init(options); err != nil {
		return err
	}
	if err := networkclientpool.Init(options); err != nil {
		return err
	}
	if err := rdapclientpool.Init(options); err != nil {
		return err
	}
	if err := compiler.Init(options); err != nil {
		return err
	}
	return nil
}

func Close() {
	protocolstate.Dialer.Close()
}
