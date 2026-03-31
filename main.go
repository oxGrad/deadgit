package main

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
	"gopkg.in/yaml.v3"
)

func main() {
	fmt.Println("deadgit - Azure DevOps Git activity scanner")
}

var (
	_ = color.New()
	_ = tablewriter.NewWriter(nil)
	_ = yaml.Decoder{}
)
