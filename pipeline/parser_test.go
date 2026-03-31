package pipeline_test

import (
	"testing"

	"github.com/oxGrad/deadgit/pipeline"
	"github.com/oxGrad/deadgit/report"
)

func TestParseExtendsPipeline_Found(t *testing.T) {
	yamlContent := `
extends:
  pipeline: shared-templates/base-pipeline.yml
  parameters:
    foo: bar
`
	info, err := pipeline.ParsePipelineFile("deploy.pipeline.yaml", []byte(yamlContent))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.FileName != "deploy.pipeline.yaml" {
		t.Errorf("expected deploy.pipeline.yaml, got %s", info.FileName)
	}
	if info.ExtendsPipeline != "shared-templates/base-pipeline.yml" {
		t.Errorf("unexpected pipeline: %s", info.ExtendsPipeline)
	}
}

func TestParseExtendsPipeline_NotFound(t *testing.T) {
	yamlContent := `
steps:
  - script: echo hello
`
	info, err := pipeline.ParsePipelineFile("simple.pipeline.yaml", []byte(yamlContent))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.ExtendsPipeline != "" {
		t.Errorf("expected empty pipeline, got %s", info.ExtendsPipeline)
	}
}

func TestParseExtendsPipeline_EmptyContent(t *testing.T) {
	info, err := pipeline.ParsePipelineFile("empty.pipeline.yaml", []byte(""))
	if err != nil {
		t.Fatalf("unexpected error on empty content: %v", err)
	}
	_ = info
}

func TestMatchesPipelineGlob(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"/pipeline/build.pipeline.yaml", true},
		{"/pipeline/deploy.pipeline.yml", true},
		{"/pipeline/build.pipeline.json", false},
		{"/other/build.pipeline.yaml", false},
		{"/pipeline/build.yaml", false},
	}
	for _, tt := range tests {
		got := pipeline.MatchesPipelineGlob(tt.path)
		if got != tt.expected {
			t.Errorf("MatchesPipelineGlob(%q) = %v, want %v", tt.path, got, tt.expected)
		}
	}
}

// compile-time check: return type is report.PipelineInfo
var _ report.PipelineInfo = report.PipelineInfo{}
