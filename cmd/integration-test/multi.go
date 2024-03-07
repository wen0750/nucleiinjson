package main

import (
	"github.com/wen0750/nucleiinjson/pkg/testutils"
)

var multiProtoTestcases = []TestCaseInfo{
	{Path: "protocols/multi/dynamic-values.yaml", TestCase: &multiProtoDynamicExtractor{}},
	{Path: "protocols/multi/evaluate-variables.yaml", TestCase: &multiProtoDynamicExtractor{}},
	{Path: "protocols/multi/exported-response-vars.yaml", TestCase: &multiProtoDynamicExtractor{}},
}

type multiProtoDynamicExtractor struct{}

// Execute executes a test case and returns an error if occurred
func (h *multiProtoDynamicExtractor) Execute(templatePath string) error {
	results, err := testutils.RunNucleiTemplateAndGetResults(templatePath, "blog.projectdiscovery.io", debug)
	if err != nil {
		return err
	}
	return expectResultsCount(results, 1)
}
