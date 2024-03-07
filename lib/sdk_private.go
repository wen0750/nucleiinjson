package nuclei

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/logrusorgru/aurora"
	"github.com/projectdiscovery/gologger"
	"github.com/projectdiscovery/gologger/levels"
	"github.com/projectdiscovery/httpx/common/httpx"
	"github.com/projectdiscovery/ratelimit"
	"github.com/wen0750/nucleiinjson/internal/runner"
	"github.com/wen0750/nucleiinjson/pkg/catalog/config"
	"github.com/wen0750/nucleiinjson/pkg/catalog/disk"
	"github.com/wen0750/nucleiinjson/pkg/core"
	"github.com/wen0750/nucleiinjson/pkg/core/inputs"
	"github.com/wen0750/nucleiinjson/pkg/installer"
	"github.com/wen0750/nucleiinjson/pkg/output"
	"github.com/wen0750/nucleiinjson/pkg/progress"
	"github.com/wen0750/nucleiinjson/pkg/protocols"
	"github.com/wen0750/nucleiinjson/pkg/protocols/common/contextargs"
	"github.com/wen0750/nucleiinjson/pkg/protocols/common/hosterrorscache"
	"github.com/wen0750/nucleiinjson/pkg/protocols/common/interactsh"
	"github.com/wen0750/nucleiinjson/pkg/protocols/common/protocolinit"
	"github.com/wen0750/nucleiinjson/pkg/protocols/common/protocolstate"
	"github.com/wen0750/nucleiinjson/pkg/protocols/http/httpclientpool"
	"github.com/wen0750/nucleiinjson/pkg/reporting"
	"github.com/wen0750/nucleiinjson/pkg/testutils"
	"github.com/wen0750/nucleiinjson/pkg/types"
)

// applyRequiredDefaults to options
func (e *NucleiEngine) applyRequiredDefaults() {
	if e.customWriter == nil {
		mockoutput := testutils.NewMockOutputWriter(e.opts.OmitTemplate)
		mockoutput.WriteCallback = func(event *output.ResultEvent) {
			if len(e.resultCallbacks) > 0 {
				for _, callback := range e.resultCallbacks {
					if callback != nil {
						callback(event)
					}
				}
				return
			}
			sb := strings.Builder{}
			sb.WriteString(fmt.Sprintf("[%v] ", event.TemplateID))
			if event.Matched != "" {
				sb.WriteString(event.Matched)
			} else {
				sb.WriteString(event.Host)
			}
			fmt.Println(sb.String())
		}
		if e.onFailureCallback != nil {
			mockoutput.FailureCallback = e.onFailureCallback
		}
		e.customWriter = mockoutput
	}
	if e.customProgress == nil {
		e.customProgress = &testutils.MockProgressClient{}
	}
	if e.hostErrCache == nil {
		e.hostErrCache = hosterrorscache.New(30, hosterrorscache.DefaultMaxHostsCount, nil)
	}
	// setup interactsh
	if e.interactshOpts != nil {
		e.interactshOpts.Output = e.customWriter
		e.interactshOpts.Progress = e.customProgress
	} else {
		e.interactshOpts = interactsh.DefaultOptions(e.customWriter, e.rc, e.customProgress)
	}
	if e.rateLimiter == nil {
		e.rateLimiter = ratelimit.New(context.Background(), 150, time.Second)
	}
	if e.opts.ExcludeTags == nil {
		e.opts.ExcludeTags = []string{}
	}
	// these templates are known to have weak matchers
	// and idea is to disable them to avoid false positives
	e.opts.ExcludeTags = append(e.opts.ExcludeTags, config.ReadIgnoreFile().Tags...)

	e.inputProvider = &inputs.SimpleInputProvider{
		Inputs: []*contextargs.MetaInput{},
	}
}

// init
func (e *NucleiEngine) init() error {
	if e.opts.Verbose {
		gologger.DefaultLogger.SetMaxLevel(levels.LevelVerbose)
	} else if e.opts.Debug {
		gologger.DefaultLogger.SetMaxLevel(levels.LevelDebug)
	} else if e.opts.Silent {
		gologger.DefaultLogger.SetMaxLevel(levels.LevelSilent)
	}

	if err := runner.ValidateOptions(e.opts); err != nil {
		return err
	}

	if e.opts.ProxyInternal && types.ProxyURL != "" || types.ProxySocksURL != "" {
		httpclient, err := httpclientpool.Get(e.opts, &httpclientpool.Configuration{})
		if err != nil {
			return err
		}
		e.httpClient = httpclient
	}

	_ = protocolstate.Init(e.opts)
	_ = protocolinit.Init(e.opts)
	e.applyRequiredDefaults()
	var err error

	// setup progressbar
	if e.enableStats {
		progressInstance, progressErr := progress.NewStatsTicker(e.opts.StatsInterval, e.enableStats, e.opts.StatsJSON, false, e.opts.MetricsPort)
		if progressErr != nil {
			return err
		}
		e.customProgress = progressInstance
		e.interactshOpts.Progress = progressInstance
	}

	if err := reporting.CreateConfigIfNotExists(); err != nil {
		return err
	}
	// we don't support reporting config in sdk mode
	if e.rc, err = reporting.New(&reporting.Options{}, ""); err != nil {
		return err
	}
	e.interactshOpts.IssuesClient = e.rc
	if e.httpClient != nil {
		e.interactshOpts.HTTPClient = e.httpClient
	}
	if e.interactshClient, err = interactsh.New(e.interactshOpts); err != nil {
		return err
	}

	e.catalog = disk.NewCatalog(config.DefaultConfig.TemplatesDirectory)

	e.executerOpts = protocols.ExecutorOptions{
		Output:          e.customWriter,
		Options:         e.opts,
		Progress:        e.customProgress,
		Catalog:         e.catalog,
		IssuesClient:    e.rc,
		RateLimiter:     e.rateLimiter,
		Interactsh:      e.interactshClient,
		HostErrorsCache: e.hostErrCache,
		Colorizer:       aurora.NewAurora(true),
		ResumeCfg:       types.NewResumeCfg(),
		Browser:         e.browserInstance,
	}

	if e.opts.RateLimitMinute > 0 {
		e.executerOpts.RateLimiter = ratelimit.New(context.Background(), uint(e.opts.RateLimitMinute), time.Minute)
	} else if e.opts.RateLimit > 0 {
		e.executerOpts.RateLimiter = ratelimit.New(context.Background(), uint(e.opts.RateLimit), time.Second)
	} else {
		e.executerOpts.RateLimiter = ratelimit.NewUnlimited(context.Background())
	}

	e.engine = core.New(e.opts)
	e.engine.SetExecuterOptions(e.executerOpts)

	httpxOptions := httpx.DefaultOptions
	httpxOptions.Timeout = 5 * time.Second
	if e.httpxClient, err = httpx.New(&httpxOptions); err != nil {
		return err
	}

	// Only Happens once regardless how many times this function is called
	// This will update ignore file to filter out templates with weak matchers to avoid false positives
	// and also upgrade templates to latest version if available
	installer.NucleiSDKVersionCheck()

	return e.processUpdateCheckResults()
}

type syncOnce struct {
	sync.Once
}

var updateCheckInstance = &syncOnce{}

// processUpdateCheckResults processes update check results
func (e *NucleiEngine) processUpdateCheckResults() error {
	var err error
	updateCheckInstance.Do(func() {
		if e.onUpdateAvailableCallback != nil {
			e.onUpdateAvailableCallback(config.DefaultConfig.LatestNucleiTemplatesVersion)
		}
		tm := installer.TemplateManager{}
		err = tm.UpdateIfOutdated()
	})
	return err
}
