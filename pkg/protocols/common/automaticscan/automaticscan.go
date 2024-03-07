package automaticscan

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/logrusorgru/aurora"
	"github.com/pkg/errors"
	"github.com/projectdiscovery/gologger"
	"github.com/projectdiscovery/retryablehttp-go"
	"github.com/projectdiscovery/useragent"
	mapsutil "github.com/projectdiscovery/utils/maps"
	sliceutil "github.com/projectdiscovery/utils/slice"
	stringsutil "github.com/projectdiscovery/utils/strings"
	wappalyzer "github.com/projectdiscovery/wappalyzergo"
	"github.com/remeh/sizedwaitgroup"
	"github.com/wen0750/nucleiinjson/pkg/catalog/config"
	"github.com/wen0750/nucleiinjson/pkg/catalog/loader"
	"github.com/wen0750/nucleiinjson/pkg/core"
	"github.com/wen0750/nucleiinjson/pkg/core/inputs"
	"github.com/wen0750/nucleiinjson/pkg/output"
	"github.com/wen0750/nucleiinjson/pkg/protocols"
	"github.com/wen0750/nucleiinjson/pkg/protocols/common/contextargs"
	"github.com/wen0750/nucleiinjson/pkg/protocols/common/helpers/writer"
	"github.com/wen0750/nucleiinjson/pkg/protocols/http/httpclientpool"
	httputil "github.com/wen0750/nucleiinjson/pkg/protocols/utils/http"
	"github.com/wen0750/nucleiinjson/pkg/scan"
	"github.com/wen0750/nucleiinjson/pkg/templates"
	"github.com/wen0750/nucleiinjson/pkg/testutils"
	"gopkg.in/yaml.v2"
)

const (
	mappingFilename = "wappalyzer-mapping.yml"
	maxDefaultBody  = 4 * 1024 * 1024 // 4MB
)

// Options contains configuration options for automatic scan service
type Options struct {
	ExecuterOpts protocols.ExecutorOptions
	Store        *loader.Store
	Engine       *core.Engine
	Target       core.InputProvider
}

// Service is a service for automatic scan execution
type Service struct {
	opts               protocols.ExecutorOptions
	store              *loader.Store
	engine             *core.Engine
	target             core.InputProvider
	wappalyzer         *wappalyzer.Wappalyze
	childExecuter      *core.ChildExecuter
	httpclient         *retryablehttp.Client
	templateDirs       []string // root Template Directories
	technologyMappings map[string]string
	techTemplates      []*templates.Template
	ServiceOpts        Options
	hasResults         *atomic.Bool
}

// New takes options and returns a new automatic scan service
func New(opts Options) (*Service, error) {
	wappalyzer, err := wappalyzer.New()
	if err != nil {
		return nil, err
	}

	// load extra mapping from nuclei-templates for normalization
	var mappingData map[string]string
	mappingFile := filepath.Join(config.DefaultConfig.GetTemplateDir(), mappingFilename)
	if file, err := os.Open(mappingFile); err == nil {
		_ = yaml.NewDecoder(file).Decode(&mappingData)
		file.Close()
	}
	if opts.ExecuterOpts.Options.Verbose {
		gologger.Verbose().Msgf("Normalized mapping (%d): %v\n", len(mappingData), mappingData)
	}

	// get template directories
	templateDirs, err := getTemplateDirs(opts)
	if err != nil {
		return nil, err
	}

	// load tech detect templates
	techDetectTemplates, err := LoadTemplatesWithTags(opts, templateDirs, []string{"tech", "detect", "favicon"}, true)
	if err != nil {
		return nil, err
	}

	childExecuter := opts.Engine.ChildExecuter()
	httpclient, err := httpclientpool.Get(opts.ExecuterOpts.Options, &httpclientpool.Configuration{
		Connection: &httpclientpool.ConnectionConfiguration{
			DisableKeepAlive: httputil.ShouldDisableKeepAlive(opts.ExecuterOpts.Options),
		},
	})
	if err != nil {
		return nil, errors.Wrap(err, "could not get http client")
	}
	return &Service{
		opts:               opts.ExecuterOpts,
		store:              opts.Store,
		engine:             opts.Engine,
		target:             opts.Target,
		wappalyzer:         wappalyzer,
		templateDirs:       templateDirs, // fix this
		childExecuter:      childExecuter,
		httpclient:         httpclient,
		technologyMappings: mappingData,
		techTemplates:      techDetectTemplates,
		ServiceOpts:        opts,
		hasResults:         &atomic.Bool{},
	}, nil
}

// Close closes the service
func (s *Service) Close() bool {
	return s.hasResults.Load()
}

// Execute automatic scan on each target with -bs host concurrency
func (s *Service) Execute() error {
	gologger.Info().Msgf("Executing Automatic scan on %d target[s]", s.target.Count())
	// setup host concurrency
	sg := sizedwaitgroup.New(s.opts.Options.BulkSize)
	s.target.Scan(func(value *contextargs.MetaInput) bool {
		sg.Add()
		go func(input *contextargs.MetaInput) {
			defer sg.Done()
			s.executeAutomaticScanOnTarget(input)
		}(value)
		return true
	})
	sg.Wait()
	return nil
}

// executeAutomaticScanOnTarget executes automatic scan on given target
func (s *Service) executeAutomaticScanOnTarget(input *contextargs.MetaInput) {
	// get tags using wappalyzer
	tagsFromWappalyzer := s.getTagsUsingWappalyzer(input)
	// get tags using detection templates
	tagsFromDetectTemplates, matched := s.getTagsUsingDetectionTemplates(input)
	if matched > 0 {
		s.hasResults.Store(true)
	}

	// create combined final tags
	finalTags := []string{}
	for _, tags := range append(tagsFromWappalyzer, tagsFromDetectTemplates...) {
		if stringsutil.EqualFoldAny(tags, "tech", "waf", "favicon") {
			continue
		}
		finalTags = append(finalTags, tags)
	}
	finalTags = sliceutil.Dedupe(finalTags)

	gologger.Info().Msgf("Found %d tags and %d matches on detection templates on %v [wappalyzer: %d, detection: %d]\n", len(finalTags), matched, input.Input, len(tagsFromWappalyzer), len(tagsFromDetectTemplates))

	// also include any extra tags passed by user
	finalTags = append(finalTags, s.opts.Options.Tags...)
	finalTags = sliceutil.Dedupe(finalTags)

	if len(finalTags) == 0 {
		gologger.Warning().Msgf("Skipping automatic scan since no tags were found on %v\n", input.Input)
		return
	}
	if s.opts.Options.VerboseVerbose {
		gologger.Print().Msgf("Final tags identified for %v: %+v\n", input.Input, finalTags)
	}

	finalTemplates, err := LoadTemplatesWithTags(s.ServiceOpts, s.templateDirs, finalTags, false)
	if err != nil {
		gologger.Error().Msgf("%v Error loading templates: %s\n", input.Input, err)
		return
	}
	gologger.Info().Msgf("Executing %d templates on %v", len(finalTemplates), input.Input)
	eng := core.New(s.opts.Options)
	execOptions := s.opts.Copy()
	execOptions.Progress = &testutils.MockProgressClient{} // stats are not supported yet due to centralized logic and cannot be reinitialized
	eng.SetExecuterOptions(execOptions)
	tmp := eng.ExecuteScanWithOpts(finalTemplates, &inputs.SimpleInputProvider{Inputs: []*contextargs.MetaInput{input}}, true)
	s.hasResults.Store(tmp.Load())
}

// getTagsUsingWappalyzer returns tags using wappalyzer by fingerprinting target
// and utilizing the mapping data
func (s *Service) getTagsUsingWappalyzer(input *contextargs.MetaInput) []string {
	req, err := retryablehttp.NewRequest(http.MethodGet, input.Input, nil)
	if err != nil {
		return nil
	}
	userAgent := useragent.PickRandom()
	req.Header.Set("User-Agent", userAgent.Raw)

	resp, err := s.httpclient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxDefaultBody))
	if err != nil {
		return nil
	}

	// fingerprint headers and body
	fingerprints := s.wappalyzer.Fingerprint(resp.Header, data)
	normalized := make(map[string]struct{})
	for k := range fingerprints {
		normalized[normalizeAppName(k)] = struct{}{}
	}
	gologger.Verbose().Msgf("Found %d fingerprints for %s\n", len(normalized), input.Input)

	// normalize fingerprints using mapping data
	for k := range normalized {
		// Replace values with mapping data
		if value, ok := s.technologyMappings[k]; ok {
			delete(normalized, k)
			normalized[value] = struct{}{}
		}
	}
	// more post processing
	items := make([]string, 0, len(normalized))
	for k := range normalized {
		if strings.Contains(k, " ") {
			parts := strings.Split(strings.ToLower(k), " ")
			items = append(items, parts...)
		} else {
			items = append(items, strings.ToLower(k))
		}
	}
	return sliceutil.Dedupe(items)
}

// getTagsUsingDetectionTemplates returns tags using detection templates
func (s *Service) getTagsUsingDetectionTemplates(input *contextargs.MetaInput) ([]string, int) {
	ctxArgs := contextargs.NewWithInput(input.Input)

	// execute tech detection templates on target
	tags := map[string]struct{}{}
	m := &sync.Mutex{}
	sg := sizedwaitgroup.New(s.opts.Options.TemplateThreads)
	counter := atomic.Uint32{}

	for _, t := range s.techTemplates {
		sg.Add()
		go func(template *templates.Template) {
			defer sg.Done()
			ctx := scan.NewScanContext(ctxArgs)
			ctx.OnResult = func(event *output.InternalWrappedEvent) {
				if event == nil {
					return
				}
				if event.HasOperatorResult() {
					// match found
					// find unique tags
					m.Lock()
					for _, v := range event.Results {
						if v.MatcherName != "" {
							tags[v.MatcherName] = struct{}{}
						}
						for _, tag := range v.Info.Tags.ToSlice() {
							// we shouldn't add all tags since tags also contain protocol type tags
							// and are not just limited to products or technologies
							// ex:   tags: js,mssql,detect,network

							// A good trick for this is check if tag is present in template-id
							if !strings.Contains(template.ID, tag) && !strings.Contains(strings.ToLower(template.Info.Name), tag) {
								// unlikely this is relevant
								continue
							}
							if _, ok := tags[tag]; !ok {
								tags[tag] = struct{}{}
							}
							// matcher names are also relevant in tech detection templates (ex: tech-detect)
							for k := range event.OperatorsResult.Matches {
								if _, ok := tags[k]; !ok {
									tags[k] = struct{}{}
								}
							}
						}
					}
					m.Unlock()
					_ = counter.Add(1)

					// TBD: should we show or hide tech detection results? what about matcher-status flag?
					_ = writer.WriteResult(event, s.opts.Output, s.opts.Progress, s.opts.IssuesClient)
				}
			}

			_, err := template.Executer.ExecuteWithResults(ctx)
			if err != nil {
				gologger.Verbose().Msgf("[%s] error executing template: %s\n", aurora.BrightYellow(template.ID), err)
				return
			}
		}(t)
	}
	sg.Wait()
	return mapsutil.GetKeys(tags), int(counter.Load())
}

// normalizeAppName normalizes app name
func normalizeAppName(appName string) string {
	if strings.Contains(appName, ":") {
		if parts := strings.Split(appName, ":"); len(parts) == 2 {
			appName = parts[0]
		}
	}
	return strings.ToLower(appName)
}
