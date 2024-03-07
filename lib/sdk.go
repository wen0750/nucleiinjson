package nuclei

import (
	"bufio"
	"bytes"
	"io"

	"github.com/projectdiscovery/httpx/common/httpx"
	"github.com/projectdiscovery/ratelimit"
	"github.com/projectdiscovery/retryablehttp-go"
	errorutil "github.com/projectdiscovery/utils/errors"
	"github.com/wen0750/nucleiinjson/pkg/catalog/disk"
	"github.com/wen0750/nucleiinjson/pkg/catalog/loader"
	"github.com/wen0750/nucleiinjson/pkg/core"
	"github.com/wen0750/nucleiinjson/pkg/core/inputs"
	"github.com/wen0750/nucleiinjson/pkg/output"
	"github.com/wen0750/nucleiinjson/pkg/parsers"
	"github.com/wen0750/nucleiinjson/pkg/progress"
	"github.com/wen0750/nucleiinjson/pkg/protocols"
	"github.com/wen0750/nucleiinjson/pkg/protocols/common/hosterrorscache"
	"github.com/wen0750/nucleiinjson/pkg/protocols/common/interactsh"
	"github.com/wen0750/nucleiinjson/pkg/protocols/headless/engine"
	"github.com/wen0750/nucleiinjson/pkg/reporting"
	"github.com/wen0750/nucleiinjson/pkg/templates"
	"github.com/wen0750/nucleiinjson/pkg/templates/signer"
	"github.com/wen0750/nucleiinjson/pkg/types"
)

// NucleiSDKOptions contains options for nuclei SDK
type NucleiSDKOptions func(e *NucleiEngine) error

var (
	// ErrNotImplemented is returned when a feature is not implemented
	ErrNotImplemented = errorutil.New("Not implemented")
	// ErrNoTemplatesAvailable is returned when no templates are available to execute
	ErrNoTemplatesAvailable = errorutil.New("No templates available")
	// ErrNoTargetsAvailable is returned when no targets are available to scan
	ErrNoTargetsAvailable = errorutil.New("No targets available")
	// ErrOptionsNotSupported is returned when an option is not supported in thread safe mode
	ErrOptionsNotSupported = errorutil.NewWithFmt("Option %v not supported in thread safe mode")
)

type engineMode uint

const (
	singleInstance engineMode = iota
	threadSafe
)

// NucleiEngine is the Engine/Client for nuclei which
// runs scans using templates and returns results
type NucleiEngine struct {
	// user options
	resultCallbacks             []func(event *output.ResultEvent)
	onFailureCallback           func(event *output.InternalEvent)
	disableTemplatesAutoUpgrade bool
	enableStats                 bool
	onUpdateAvailableCallback   func(newVersion string)

	// ready-status fields
	templatesLoaded bool

	// unexported core fields
	interactshClient *interactsh.Client
	catalog          *disk.DiskCatalog
	rateLimiter      *ratelimit.Limiter
	store            *loader.Store
	httpxClient      *httpx.HTTPX
	inputProvider    *inputs.SimpleInputProvider
	engine           *core.Engine
	mode             engineMode
	browserInstance  *engine.Browser
	httpClient       *retryablehttp.Client

	// unexported meta options
	opts           *types.Options
	interactshOpts *interactsh.Options
	hostErrCache   *hosterrorscache.Cache
	customWriter   output.Writer
	customProgress progress.Progress
	rc             reporting.Client
	executerOpts   protocols.ExecutorOptions
}

// LoadAllTemplates loads all nuclei template based on given options
func (e *NucleiEngine) LoadAllTemplates() error {
	workflowLoader, err := parsers.NewLoader(&e.executerOpts)
	if err != nil {
		return errorutil.New("Could not create workflow loader: %s\n", err)
	}
	e.executerOpts.WorkflowLoader = workflowLoader

	e.store, err = loader.New(loader.NewConfig(e.opts, e.catalog, e.executerOpts))
	if err != nil {
		return errorutil.New("Could not create loader client: %s\n", err)
	}
	e.store.Load()
	return nil
}

// GetTemplates returns all nuclei templates that are loaded
func (e *NucleiEngine) GetTemplates() []*templates.Template {
	if !e.templatesLoaded {
		_ = e.LoadAllTemplates()
	}
	return e.store.Templates()
}

// LoadTargets(urls/domains/ips only) adds targets to the nuclei engine
func (e *NucleiEngine) LoadTargets(targets []string, probeNonHttp bool) {
	for _, target := range targets {
		if probeNonHttp {
			e.inputProvider.SetWithProbe(target, e.httpxClient)
		} else {
			e.inputProvider.Set(target)
		}
	}
}

// LoadTargetsFromReader adds targets(urls/domains/ips only) from reader to the nuclei engine
func (e *NucleiEngine) LoadTargetsFromReader(reader io.Reader, probeNonHttp bool) {
	buff := bufio.NewScanner(reader)
	for buff.Scan() {
		if probeNonHttp {
			e.inputProvider.SetWithProbe(buff.Text(), e.httpxClient)
		} else {
			e.inputProvider.Set(buff.Text())
		}
	}
}

// GetExecuterOptions returns the nuclei executor options
func (e *NucleiEngine) GetExecuterOptions() *protocols.ExecutorOptions {
	return &e.executerOpts
}

// ParseTemplate parses a template from given data
// template verification status can be accessed from template.Verified
func (e *NucleiEngine) ParseTemplate(data []byte) (*templates.Template, error) {
	return templates.ParseTemplateFromReader(bytes.NewReader(data), nil, e.executerOpts)
}

// SignTemplate signs the tempalate using given signer
func (e *NucleiEngine) SignTemplate(tmplSigner *signer.TemplateSigner, data []byte) ([]byte, error) {
	tmpl, err := e.ParseTemplate(data)
	if err != nil {
		return data, err
	}
	if tmpl.Verified {
		// already signed
		return data, nil
	}
	if len(tmpl.Workflows) > 0 {
		return data, templates.ErrNotATemplate
	}
	signatureData, err := tmplSigner.Sign(data, tmpl)
	if err != nil {
		return data, err
	}
	buff := bytes.NewBuffer(signer.RemoveSignatureFromData(data))
	buff.WriteString("\n" + signatureData)
	return buff.Bytes(), err
}

// Close all resources used by nuclei engine
func (e *NucleiEngine) Close() {
	e.interactshClient.Close()
	e.rc.Close()
	e.customWriter.Close()
	e.hostErrCache.Close()
	e.executerOpts.RateLimiter.Stop()
}

// ExecuteWithCallback executes templates on targets and calls callback on each result(only if results are found)
func (e *NucleiEngine) ExecuteWithCallback(callback ...func(event *output.ResultEvent)) error {
	if !e.templatesLoaded {
		_ = e.LoadAllTemplates()
	}
	if len(e.store.Templates()) == 0 && len(e.store.Workflows()) == 0 {
		return ErrNoTemplatesAvailable
	}
	if e.inputProvider.Count() == 0 {
		return ErrNoTargetsAvailable
	}

	filtered := []func(event *output.ResultEvent){}
	for _, callback := range callback {
		if callback != nil {
			filtered = append(filtered, callback)
		}
	}
	e.resultCallbacks = append(e.resultCallbacks, filtered...)

	_ = e.engine.ExecuteScanWithOpts(e.store.Templates(), e.inputProvider, false)
	defer e.engine.WorkPool().Wait()
	return nil
}

// NewNucleiEngine creates a new nuclei engine instance
func NewNucleiEngine(options ...NucleiSDKOptions) (*NucleiEngine, error) {
	// default options
	e := &NucleiEngine{
		opts: types.DefaultOptions(),
		mode: singleInstance,
	}
	for _, option := range options {
		if err := option(e); err != nil {
			return nil, err
		}
	}
	if err := e.init(); err != nil {
		return nil, err
	}
	return e, nil
}
