package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cloudfoundry/cli/plugin"
)

type CliCmd struct{}

//GetMetadata returns metatada
func (c *CliCmd) GetMetadata() plugin.PluginMetadata {
	return plugin.PluginMetadata{
		Name: "scaleover",
		Version: plugin.VersionType{
			Major: 0,
			Minor: 0,
			Build: 1,
		},
		Commands: []plugin.Command{
			{
				Name:     "scaleover",
				HelpText: "Roll http traffic from one application to another",
				UsageDetails: plugin.Usage{
					Usage: "cf scaleover APP1 APP2 TIME",
				},
			},
		},
	}
}

func main() {
	plugin.Start(new(CliCmd))
}

func (c *CliCmd) Run(cliConnection plugin.CliConnection, args []string) {
	var firstAppInstanceCount int

	//TODO Check for apps!
	rolloverTime, _ := strconv.Atoi(args[3])
	app1 := args[1]
	app2 := args[2]

	fmt.Println("Starting scaleover of", app1, "to", app2)
	output, _ := cliConnection.CliCommandWithoutTerminalOutput(
		"scale", app1)
	for _, v := range output {
		if strings.HasPrefix(v, "instances:") {
			tmp := strings.TrimSpace(strings.TrimPrefix(v, "instances: "))
			firstAppInstanceCount, _ = strconv.Atoi(tmp)
		}
	}

	sleepInterval := rolloverTime / firstAppInstanceCount

	fmt.Println("starting", app2)
	cliConnection.CliCommandWithoutTerminalOutput(
		"scale", "-i", strconv.Itoa(1), app2)
	cliConnection.CliCommandWithoutTerminalOutput("start", app2)
	fmt.Println(app2, "is started")

	for i := firstAppInstanceCount - 1; i > 0; i-- {
		cliConnection.CliCommandWithoutTerminalOutput(
			"scale", "-i", strconv.Itoa(i), app1)

		app2Instances := strconv.Itoa(firstAppInstanceCount - i + 1)
		cliConnection.CliCommandWithoutTerminalOutput(
			"scale", "-i", app2Instances, app2)

		fmt.Println("Scaled", app1, "to", i, app2, "to", app2Instances)
		time.Sleep(time.Duration(sleepInterval) * time.Second)
	}

	fmt.Println("Stopping", app1)
	cliConnection.CliCommandWithoutTerminalOutput("stop", app1)
	fmt.Println("Scaleover complete!")

}
