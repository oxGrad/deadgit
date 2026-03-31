package pipeline

import (
	"path"
	"strings"

	"github.com/oxGrad/deadgit/report"
	"gopkg.in/yaml.v3"
)

type pipelineYAML struct {
	Extends struct {
		Pipeline string `yaml:"pipeline"`
	} `yaml:"extends"`
}

// ParsePipelineFile parses raw YAML content and returns a PipelineInfo.
// fileName is the base name of the file (e.g. "build.pipeline.yaml").
func ParsePipelineFile(fileName string, content []byte) (report.PipelineInfo, error) {
	info := report.PipelineInfo{FileName: fileName}
	if len(content) == 0 {
		return info, nil
	}
	var doc pipelineYAML
	if err := yaml.Unmarshal(content, &doc); err != nil {
		// Return partial info rather than failing hard
		return info, nil
	}
	info.ExtendsPipeline = doc.Extends.Pipeline
	return info, nil
}

// MatchesPipelineGlob returns true if the item path looks like
// /pipeline/*.pipeline.yaml or /pipeline/*.pipeline.yml
func MatchesPipelineGlob(itemPath string) bool {
	dir := path.Dir(itemPath)
	if dir != "/pipeline" {
		return false
	}
	base := path.Base(itemPath)
	matched, _ := path.Match("*.pipeline.yaml", base)
	if matched {
		return true
	}
	matched, _ = path.Match("*.pipeline.yml", base)
	return matched
}

// ExtractFileName returns the base name from a full path.
func ExtractFileName(itemPath string) string {
	parts := strings.Split(itemPath, "/")
	return parts[len(parts)-1]
}
