package multiproto_test

import (
	"context"
	"log"
	"testing"
	"time"

	"github.com/projectdiscovery/ratelimit"
	"github.com/stretchr/testify/require"
	"github.com/wen0750/nucleiinjson/pkg/catalog/config"
	"github.com/wen0750/nucleiinjson/pkg/catalog/disk"
	"github.com/wen0750/nucleiinjson/pkg/parsers"
	"github.com/wen0750/nucleiinjson/pkg/progress"
	"github.com/wen0750/nucleiinjson/pkg/protocols"
	"github.com/wen0750/nucleiinjson/pkg/protocols/common/contextargs"
	"github.com/wen0750/nucleiinjson/pkg/scan"
	"github.com/wen0750/nucleiinjson/pkg/templates"
	"github.com/wen0750/nucleiinjson/pkg/testutils"
)

var executerOpts protocols.ExecutorOptions

func setup() {
	options := testutils.DefaultOptions
	testutils.Init(options)
	progressImpl, _ := progress.NewStatsTicker(0, false, false, false, 0)

	executerOpts = protocols.ExecutorOptions{
		Output:       testutils.NewMockOutputWriter(options.OmitTemplate),
		Options:      options,
		Progress:     progressImpl,
		ProjectFile:  nil,
		IssuesClient: nil,
		Browser:      nil,
		Catalog:      disk.NewCatalog(config.DefaultConfig.TemplatesDirectory),
		RateLimiter:  ratelimit.New(context.Background(), uint(options.RateLimit), time.Second),
	}
	workflowLoader, err := parsers.NewLoader(&executerOpts)
	if err != nil {
		log.Fatalf("Could not create workflow loader: %s\n", err)
	}
	executerOpts.WorkflowLoader = workflowLoader
}

func TestMultiProtoWithDynamicExtractor(t *testing.T) {
	setup()
	Template, err := templates.Parse("testcases/multiprotodynamic.yaml", nil, executerOpts)
	require.Nil(t, err, "could not parse template")

	require.Equal(t, 2, len(Template.RequestsQueue))

	err = Template.Executer.Compile()
	require.Nil(t, err, "could not compile template")

	input := contextargs.NewWithInput("blog.projectdiscovery.io")
	ctx := scan.NewScanContext(input)
	gotresults, err := Template.Executer.Execute(ctx)
	require.Nil(t, err, "could not execute template")
	require.True(t, gotresults)
}

func TestMultiProtoWithProtoPrefix(t *testing.T) {
	setup()
	Template, err := templates.Parse("testcases/multiprotowithprefix.yaml", nil, executerOpts)
	require.Nil(t, err, "could not parse template")

	require.Equal(t, 3, len(Template.RequestsQueue))

	err = Template.Executer.Compile()
	require.Nil(t, err, "could not compile template")

	input := contextargs.NewWithInput("blog.projectdiscovery.io")
	ctx := scan.NewScanContext(input)
	gotresults, err := Template.Executer.Execute(ctx)
	require.Nil(t, err, "could not execute template")
	require.True(t, gotresults)
}
