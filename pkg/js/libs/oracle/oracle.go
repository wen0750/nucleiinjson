package oracle

import (
	"context"
	"net"
	"strconv"
	"time"

	"github.com/praetorian-inc/fingerprintx/pkg/plugins"
	"github.com/praetorian-inc/fingerprintx/pkg/plugins/services/oracledb"
	"github.com/wen0750/nucleiinjson/pkg/protocols/common/protocolstate"
)

// OracleClient is a minimal Oracle client for nuclei scripts.
type OracleClient struct{}

// IsOracleResponse is the response from the IsOracle function.
type IsOracleResponse struct {
	IsOracle bool
	Banner   string
}

// IsOracle checks if a host is running an Oracle server.
func (c *OracleClient) IsOracle(host string, port int) (IsOracleResponse, error) {
	resp := IsOracleResponse{}

	timeout := 5 * time.Second
	conn, err := protocolstate.Dialer.Dial(context.TODO(), "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return resp, err
	}
	defer conn.Close()

	oracledbPlugin := oracledb.ORACLEPlugin{}
	service, err := oracledbPlugin.Run(conn, timeout, plugins.Target{Host: host})
	if err != nil {
		return resp, err
	}
	if service == nil {
		return resp, nil
	}
	resp.Banner = service.Version
	resp.Banner = service.Metadata().(plugins.ServiceOracle).Info
	resp.IsOracle = true
	return resp, nil
}
